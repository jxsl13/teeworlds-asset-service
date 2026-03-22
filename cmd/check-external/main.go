package main

import (
	"fmt"
	"image/png"
	"os"

	"github.com/jxsl13/teeworlds-asset-service/internal/twmap"
)

func main() {
	f, err := os.Open(os.Args[1])
	if err != nil {
		fmt.Fprintln(os.Stderr, err)
		os.Exit(1)
	}
	defer f.Close()
	m, err := twmap.Parse(f)
	if err != nil {
		fmt.Fprintln(os.Stderr, "parse error:", err)
		os.Exit(1)
	}

	fmt.Println("=== Images ===")
	for i, img := range m.Images {
		fmt.Printf("  img[%d] name=%-25s ext=%-5v pixels=%-5v %dx%d\n", i, img.Name, img.External, img.RGBA != nil, img.Width, img.Height)
	}

	fmt.Println("\n=== Groups ===")
	for gi, g := range m.Groups {
		fmt.Printf("  group[%d] name=%-15s parallax=%d/%d offset=%d/%d clip=%v layers=%d\n",
			gi, g.Name, g.ParallaxX, g.ParallaxY, g.OffsetX, g.OffsetY, g.Clipping, len(g.Layers))

		skippable := g.ParallaxX != 100 || g.ParallaxY != 100 || g.OffsetX != 0 || g.OffsetY != 0 || g.Clipping
		if skippable {
			fmt.Printf("           -> SKIPPED by renderer (parallax/offset/clip)\n")
		}

		for li, l := range g.Layers {
			physics := l.IsPhysics()
			kind := "?"
			switch l.Kind {
			case twmap.LayerKindTiles:
				kind = "tiles"
			case twmap.LayerKindGame:
				kind = "game"
			case twmap.LayerKindFront:
				kind = "front"
			case twmap.LayerKindTele:
				kind = "tele"
			case twmap.LayerKindSpeedup:
				kind = "speedup"
			case twmap.LayerKindSwitch:
				kind = "switch"
			case twmap.LayerKindTune:
				kind = "tune"
			case twmap.LayerKindQuads:
				kind = "quads"
			case twmap.LayerKindSounds:
				kind = "sounds"
			}

			renderable := !skippable && !physics && !l.Detail
			nonAir := 0
			for _, t := range l.Tiles {
				if t.ID != 0 {
					nonAir++
				}
			}

			fmt.Printf("    layer[%d] kind=%-8s detail=%-5v physics=%-5v imgID=%-3d tiles=%d nonAir=%d quads=%d renderable=%v\n",
				li, kind, l.Detail, physics, l.ImageID, len(l.Tiles), nonAir, len(l.Quads), renderable)
		}
	}

	// Try rendering
	fmt.Println("\n=== Render ===")
	img, err := twmap.RenderMap(m, 512, 512)
	if err != nil {
		fmt.Printf("  render error: %v\n", err)
		os.Exit(1)
	}
	fmt.Printf("  result: %dx%d\n", img.Bounds().Dx(), img.Bounds().Dy())

	if len(os.Args) > 2 {
		out, err := os.Create(os.Args[2])
		if err != nil {
			fmt.Fprintln(os.Stderr, err)
			os.Exit(1)
		}
		defer out.Close()
		png.Encode(out, img)
		fmt.Printf("  saved to %s\n", os.Args[2])
	}
}
