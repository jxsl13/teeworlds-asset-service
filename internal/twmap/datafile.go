// Package twmap implements parsing, validation and thumbnail generation for
// Teeworlds 0.6.x and DDNet map files.
//
// The Teeworlds map format is a "datafile" container holding typed items
// (metadata) and compressed data blocks (tile data, image data, etc.).
//
// Reference: https://gitlab.com/ddnet-rs/twmap
package twmap

import (
	"bytes"
	"encoding/binary"
	"fmt"
	"io"

	"github.com/klauspost/compress/zlib"
)

// ── datafile magic & versions ────────────────────────────────────────────────

var (
	magic          = [4]byte{'D', 'A', 'T', 'A'}
	magicBigEndian = [4]byte{'A', 'T', 'A', 'D'}
)

const (
	datafileVersion3 = 3
	datafileVersion4 = 4
)

// ── datafile header structs ──────────────────────────────────────────────────

type headerVersion struct {
	Magic   [4]byte
	Version int32
}

type headerRest struct {
	Size         int32
	Swaplen      int32
	NumItemTypes int32
	NumItems     int32
	NumData      int32
	SizeItems    int32
	SizeData     int32
}

type itemType struct {
	TypeID int32
	Start  int32
	Num    int32
}

type itemHeader struct {
	TypeIDAndID int32
	Size        int32
}

func (h itemHeader) typeID() uint16 {
	return uint16((uint32(h.TypeIDAndID) >> 16) & 0xFFFF)
}

func (h itemHeader) id() uint16 {
	return uint16(uint32(h.TypeIDAndID) & 0xFFFF)
}

// ── item view ────────────────────────────────────────────────────────────────

type itemView struct {
	TypeID uint16
	ID     uint16
	Data   []int32
}

// ── datafile reader ──────────────────────────────────────────────────────────

type datafile struct {
	version   int32
	itemTypes []itemType
	items     []itemView
	numData   int
	// raw data offsets/sizes for lazy decompression
	dataOffsets []int32
	dataSizes   []int32 // only version 4
	rawData     []byte  // the compressed data section
}

// parseDatafile reads and validates the datafile container from r.
func parseDatafile(r io.Reader) (*datafile, error) {
	// ── version header ───────────────────────────────────────────────────
	var hv headerVersion
	if err := binary.Read(r, binary.LittleEndian, &hv); err != nil {
		return nil, fmt.Errorf("reading version header: %w", err)
	}
	if hv.Magic != magic && hv.Magic != magicBigEndian {
		return nil, fmt.Errorf("wrong magic: %x", hv.Magic)
	}
	if hv.Version != datafileVersion3 && hv.Version != datafileVersion4 {
		return nil, fmt.Errorf("unsupported datafile version: %d", hv.Version)
	}

	// ── header rest ──────────────────────────────────────────────────────
	var hr headerRest
	if err := binary.Read(r, binary.LittleEndian, &hr); err != nil {
		return nil, fmt.Errorf("reading header: %w", err)
	}
	if err := checkHeader(&hr); err != nil {
		return nil, err
	}

	// ── item types ───────────────────────────────────────────────────────
	itTypes := make([]itemType, hr.NumItemTypes)
	if err := binary.Read(r, binary.LittleEndian, itTypes); err != nil {
		return nil, fmt.Errorf("reading item types: %w", err)
	}

	// ── item offsets ─────────────────────────────────────────────────────
	itemOffsets := make([]int32, hr.NumItems)
	if err := binary.Read(r, binary.LittleEndian, itemOffsets); err != nil {
		return nil, fmt.Errorf("reading item offsets: %w", err)
	}

	// ── data offsets ─────────────────────────────────────────────────────
	dataOffsets := make([]int32, hr.NumData)
	if err := binary.Read(r, binary.LittleEndian, dataOffsets); err != nil {
		return nil, fmt.Errorf("reading data offsets: %w", err)
	}

	// ── data sizes (version 4 only) ──────────────────────────────────────
	var dataSizes []int32
	if hv.Version >= datafileVersion4 {
		dataSizes = make([]int32, hr.NumData)
		if err := binary.Read(r, binary.LittleEndian, dataSizes); err != nil {
			return nil, fmt.Errorf("reading data sizes: %w", err)
		}
	}

	// ── items block ──────────────────────────────────────────────────────
	itemsRaw := make([]byte, hr.SizeItems)
	if _, err := io.ReadFull(r, itemsRaw); err != nil {
		return nil, fmt.Errorf("reading items block: %w", err)
	}

	// ── data block ───────────────────────────────────────────────────────
	dataRaw := make([]byte, hr.SizeData)
	if _, err := io.ReadFull(r, dataRaw); err != nil {
		return nil, fmt.Errorf("reading data block: %w", err)
	}

	// ── parse individual items ───────────────────────────────────────────
	items, err := parseItems(itemsRaw, int(hr.NumItems), itemOffsets)
	if err != nil {
		return nil, err
	}

	return &datafile{
		version:     hv.Version,
		itemTypes:   itTypes,
		items:       items,
		numData:     int(hr.NumData),
		dataOffsets: dataOffsets,
		dataSizes:   dataSizes,
		rawData:     dataRaw,
	}, nil
}

func checkHeader(hr *headerRest) error {
	if hr.NumItemTypes < 0 {
		return fmt.Errorf("negative num_asset_types: %d", hr.NumItemTypes)
	}
	if hr.NumItems < 0 {
		return fmt.Errorf("negative num_items: %d", hr.NumItems)
	}
	if hr.NumData < 0 {
		return fmt.Errorf("negative num_data: %d", hr.NumData)
	}
	if hr.SizeItems < 0 {
		return fmt.Errorf("negative size_items: %d", hr.SizeItems)
	}
	if hr.SizeData < 0 {
		return fmt.Errorf("negative size_data: %d", hr.SizeData)
	}
	if hr.SizeItems%4 != 0 {
		return fmt.Errorf("size_items not divisible by 4: %d", hr.SizeItems)
	}
	return nil
}

func parseItems(raw []byte, numItems int, offsets []int32) ([]itemView, error) {
	items := make([]itemView, 0, numItems)
	for i := range numItems {
		off := int(offsets[i])
		if off < 0 || off+8 > len(raw) {
			return nil, fmt.Errorf("item %d: offset %d out of bounds", i, off)
		}
		var ih itemHeader
		ih.TypeIDAndID = int32(binary.LittleEndian.Uint32(raw[off : off+4]))
		ih.Size = int32(binary.LittleEndian.Uint32(raw[off+4 : off+8]))
		if ih.Size < 0 || ih.Size%4 != 0 {
			return nil, fmt.Errorf("item %d: invalid size %d", i, ih.Size)
		}
		dataStart := off + 8
		dataEnd := dataStart + int(ih.Size)
		if dataEnd > len(raw) {
			return nil, fmt.Errorf("item %d: data extends beyond items block", i)
		}
		data := make([]int32, ih.Size/4)
		for j := range data {
			data[j] = int32(binary.LittleEndian.Uint32(raw[dataStart+j*4 : dataStart+j*4+4]))
		}
		items = append(items, itemView{
			TypeID: ih.typeID(),
			ID:     ih.id(),
			Data:   data,
		})
	}
	return items, nil
}

// readData decompresses and returns the data block at the given index.
func (df *datafile) readData(index int) ([]byte, error) {
	if index < 0 || index >= df.numData {
		return nil, fmt.Errorf("data index %d out of range [0, %d)", index, df.numData)
	}
	start := df.dataOffsets[index]
	var end int32
	if index+1 < df.numData {
		end = df.dataOffsets[index+1]
	} else {
		end = int32(len(df.rawData))
	}
	if start < 0 || end < start || int(end) > len(df.rawData) {
		return nil, fmt.Errorf("data index %d: invalid offset range [%d, %d)", index, start, end)
	}
	compressed := df.rawData[start:end]

	if df.version == datafileVersion3 {
		// version 3: data is stored uncompressed
		out := make([]byte, len(compressed))
		copy(out, compressed)
		return out, nil
	}

	// version 4: zlib compressed
	zr, err := zlib.NewReader(bytes.NewReader(compressed))
	if err != nil {
		return nil, fmt.Errorf("data index %d: zlib init: %w", index, err)
	}
	defer zr.Close()

	var buf bytes.Buffer
	if df.dataSizes != nil && df.dataSizes[index] > 0 {
		buf.Grow(int(df.dataSizes[index]))
	}
	if _, err := io.Copy(&buf, zr); err != nil {
		return nil, fmt.Errorf("data index %d: zlib decompress: %w", index, err)
	}
	return buf.Bytes(), nil
}

// findItem returns the first item of the given type and id, or nil.
func (df *datafile) findItem(typeID uint16, id uint16) *itemView {
	for i := range df.items {
		if df.items[i].TypeID == typeID && df.items[i].ID == id {
			return &df.items[i]
		}
	}
	return nil
}

// itemTypeIndices returns the index range of items with the given type_id.
func (df *datafile) itemTypeIndices(typeID uint16) (int, int) {
	for _, it := range df.itemTypes {
		if uint16(it.TypeID) == typeID {
			start := int(it.Start)
			end := start + int(it.Num)
			return start, end
		}
	}
	return 0, 0
}

// itemsOfType returns all items of the given type.
func (df *datafile) itemsOfType(typeID uint16) []itemView {
	start, end := df.itemTypeIndices(typeID)
	if start == end {
		return nil
	}
	if start >= len(df.items) || end > len(df.items) {
		return nil
	}
	return df.items[start:end]
}
