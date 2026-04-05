/* ══════════════════════════════════════════════════════════════════════════════
   Teeworlds Asset Database – Upload & UI logic
   ══════════════════════════════════════════════════════════════════════════════ */

/* global activeType is set by the template inline script */

/* ── CSRF helper ────────────────────────────────────────────────────────────── */
function getCSRFToken() {
  var match = document.cookie.match('(?:^|; )__csrf=([^;]*)');
  return match ? decodeURIComponent(match[1]) : '';
}

/* ── Image preview ─────────────────────────────────────────────────────────── */
function openPreview(src) {
  document.getElementById('previewImg').src = src;
  document.getElementById('previewModal').classList.add('open');
}

function closePreview() {
  document.getElementById('previewModal').classList.remove('open');
  document.getElementById('previewImg').src = '';
}

document.addEventListener('keydown', function (e) {
  if (e.key === 'Escape') { closePreview(); closeMetadataModal(); }

  // Ctrl+K → focus search input
  if ((e.ctrlKey || e.metaKey) && e.key === 'k') {
    e.preventDefault();
    var search = document.getElementById('search');
    if (search) search.focus();
  }

  // Arrow keys → pagination (only when not typing in an input/textarea)
  var tag = document.activeElement ? document.activeElement.tagName : '';
  if (tag === 'INPUT' || tag === 'TEXTAREA' || tag === 'SELECT') return;
  if (e.key === 'ArrowLeft') {
    var prev = document.getElementById('prevPage');
    if (prev && !prev.disabled) prev.click();
  }
  if (e.key === 'ArrowRight') {
    var next = document.getElementById('nextPage');
    if (next && !next.disabled) next.click();
  }
});

/* ── Tab switching ─────────────────────────────────────────────────────────── */
function switchTab(btn) {
  document.querySelectorAll('.tab').forEach(function (t) { t.classList.remove('active'); });
  btn.classList.add('active');
  activeType = btn.dataset.type;
  var search = document.getElementById('search');
  search.setAttribute('hx-get', '/' + activeType);
  search.value = '';
  htmx.process(search);
  document.getElementById('uploadTypeLabel').textContent = activeType;
  currentSort = [{field: 'name', dir: 'asc'}];
  var sortInput = document.getElementById('searchSort');
  if (sortInput) sortInput.value = 'name:asc';
  // Reset filter state when switching tabs
  htmx.ajax('GET', '/' + activeType + '?limit=' + itemsPerPage + '&offset=0&sort=name:asc', '#content');
}

/* ── Column sorting ────────────────────────────────────────────────────────── */
/*
  currentSort is an ordered array of { field, dir } objects.
  - Without Shift: clicking a column replaces the entire sort with that column.
    Clicking the same column toggles asc → desc → unsorted.
  - With Shift: clicking adds/toggles the column as a secondary sort (max 2).
*/
var currentSort = [{field: 'name', dir: 'asc'}];

/* Initialise currentSort + searchSort from the URL or the server-provided var */
(function initSortFromURL() {
  var sortParam = '';
  try {
    var params = new URLSearchParams(window.location.search);
    sortParam = params.get('sort') || '';
  } catch (_) { /* IE / about:blank */ }
  if (!sortParam && typeof initialSort !== 'undefined') sortParam = initialSort;
  if (sortParam) {
    var parts = sortParam.split(',');
    var parsed = [];
    for (var i = 0; i < parts.length; i++) {
      var pair = parts[i].split(':');
      if (pair.length === 2 && (pair[1] === 'asc' || pair[1] === 'desc')) {
        parsed.push({field: pair[0], dir: pair[1]});
      }
    }
    if (parsed.length > 0) currentSort = parsed;
  }
  var sortInput = document.getElementById('searchSort');
  if (sortInput) {
    sortInput.value = currentSort.map(function (s) { return s.field + ':' + s.dir; }).join(',');
  }
})();

function onSortClick(event, field) {
  var shift = event.shiftKey;
  var existing = -1;
  for (var i = 0; i < currentSort.length; i++) {
    if (currentSort[i].field === field) { existing = i; break; }
  }

  if (shift) {
    /* Multi-column: add or toggle this field */
    if (existing >= 0) {
      var cur = currentSort[existing].dir;
      if (cur === 'asc') {
        currentSort[existing].dir = 'desc';
      } else {
        /* Remove from sort */
        currentSort.splice(existing, 1);
      }
    } else {
      if (currentSort.length >= 2) return; /* max 2 columns */
      currentSort.push({ field: field, dir: 'asc' });
    }
  } else {
    /* Single-column: replace */
    if (existing >= 0 && currentSort.length === 1) {
      var cur = currentSort[0].dir;
      if (cur === 'asc') {
        currentSort = [{ field: field, dir: 'desc' }];
      } else {
        currentSort = []; /* clear sort */
      }
    } else {
      currentSort = [{ field: field, dir: 'asc' }];
    }
  }

  applySortRequest();
}

function applySortRequest() {
  var sortParam = currentSort.map(function (s) { return s.field + ':' + s.dir; }).join(',');
  // Keep the hidden sort input in sync so search requests include the sort.
  var sortInput = document.getElementById('searchSort');
  if (sortInput) sortInput.value = sortParam;
  var url = '/' + activeType + '?limit=' + itemsPerPage + '&offset=0';
  var search = document.getElementById('search');
  var q = search ? search.value.trim() : '';
  if (q) url += '&q=' + encodeURIComponent(q);
  if (sortParam) url += '&sort=' + encodeURIComponent(sortParam);
  htmx.ajax('GET', url, '#content');
}

/* ── Filter by a table cell value ─────────────────────────────────────────── */
/*
  filterByCell(chipEl) — called from onclick="filterByCell(this)" on .filter-chip
  spans in the items table. Clears the search query and fires a list request
  filtered to this field/value. Preserves the active sort order.
  Multiple filters can be stacked; re-clicking the same value is a no-op.
*/
function filterByCell(el) {
  var field = el.dataset.field;
  var value = el.dataset.value;
  if (!field || !value) return;

  // Clear the search input so the server runs ListItems (not fuzzy Search).
  var search = document.getElementById('search');
  if (search) search.value = '';

  // Build from current URL so existing filters are preserved.
  var params = new URLSearchParams(window.location.search);
  params.set('offset', '0');
  params.set(field, value);
  // Ensure sort param stays in sync with JS state.
  var sortInput = document.getElementById('searchSort');
  if (sortInput && sortInput.value) params.set('sort', sortInput.value);
  // Remove search query when filtering.
  params.delete('q');

  htmx.ajax('GET', '/' + activeType + '?' + params.toString(), '#content');
}

/* ── Clear active filters ──────────────────────────────────────────────────── */
function clearFilter(field) {
  var params = new URLSearchParams(window.location.search);
  params.delete(field);
  params.set('offset', '0');
  htmx.ajax('GET', '/' + activeType + '?' + params.toString(), '#content');
}

function clearAllFilters() {
  var params = new URLSearchParams(window.location.search);
  ['creator', 'license', 'date'].forEach(function (f) { params.delete(f); });
  params.set('offset', '0');
  htmx.ajax('GET', '/' + activeType + '?' + params.toString(), '#content');
}

/* ── Shared helpers ────────────────────────────────────────────────────────── */
var licenseOptions =
  '<option value="cc-by">CC Attribution</option>' +
  '<option value="cc-by-sa">CC Attribution-ShareAlike</option>' +
  '<option value="cc0">CC Zero (Public Domain)</option>' +
  '<option value="cc-by-nc">CC Attribution-NonCommercial</option>' +
  '<option value="cc-by-nc-sa">CC Attr-NonComm-ShareAlike</option>' +
  '<option value="cc-by-nd">CC Attribution-NoDerivs</option>' +
  '<option value="cc-by-nc-nd">CC Attr-NonComm-NoDerivs</option>' +
  '<option value="gpl-2">GPL v2</option>' +
  '<option value="gpl-3">GPL v3</option>' +
  '<option value="mit">MIT</option>' +
  '<option value="apache-2">Apache 2.0</option>' +
  '<option value="zlib">zlib</option>' +
  '<option value="unknown" selected>Unknown</option>';

function nameFromFile(filename) {
  return filename.replace(/\.[^/.]+$/, '').replace(/[_-]+/g, ' ').trim();
}

function escapeHtml(s) {
  var d = document.createElement('div');
  d.textContent = s;
  return d.innerHTML;
}

/* ── License matching ──────────────────────────────────────────────────────── */
/*
  matchLicense maps a free-form license string from the map info to the
  closest dropdown option value. Returns the option value or null.
*/
function matchLicense(raw) {
  var s = raw.toLowerCase().replace(/[^a-z0-9 ]/g, ' ').replace(/\s+/g, ' ').trim();
  var patterns = [
    [/\bcc\b.*\bby\b.*\bnc\b.*\bsa\b/,   'cc-by-nc-sa'],
    [/\bcc\b.*\bby\b.*\bnc\b.*\bnd\b/,   'cc-by-nc-nd'],
    [/\bcc\b.*\bby\b.*\bnc\b/,            'cc-by-nc'],
    [/\bcc\b.*\bby\b.*\bsa\b/,            'cc-by-sa'],
    [/\bcc\b.*\bby\b.*\bnd\b/,            'cc-by-nd'],
    [/\bcc\b.*\bby\b/,                    'cc-by'],
    [/\bcc\s*0\b|\bcc\b.*\bzero\b|\bpublic\s*domain\b/, 'cc0'],
    [/\bgpl\b.*\b3\b/,                    'gpl-3'],
    [/\bgpl\b.*\b2\b/,                    'gpl-2'],
    [/\bgpl\b/,                            'gpl-3'],
    [/\bmit\b/,                            'mit'],
    [/\bapache\b/,                         'apache-2'],
    [/\bzlib\b/,                           'zlib']
  ];
  for (var i = 0; i < patterns.length; i++) {
    if (patterns[i][0].test(s)) return patterns[i][1];
  }
  return null;
}

/* ── Tag input helpers ─────────────────────────────────────────────────────── */
function initTagInput(container) {
  var input = container.querySelector('input');
  input.addEventListener('keydown', function (e) {
    if (e.key === 'Enter' || e.key === ',') {
      e.preventDefault();
      commitTag(container, input);
    }
    if (e.key === 'Backspace' && input.value === '') {
      var chips = container.querySelectorAll('.tag-chip');
      if (chips.length) chips[chips.length - 1].remove();
    }
  });
  input.addEventListener('blur', function () { commitTag(container, input); });
}

function commitTag(container, input) {
  var val = input.value.replace(/,/g, '').trim();
  if (!val) return;
  var existing = getTagValues(container);
  if (existing.indexOf(val) !== -1) { input.value = ''; return; }
  var chip = document.createElement('span');
  chip.className = 'tag-chip';
  chip.innerHTML = escapeHtml(val) + '<span class="tag-remove" onclick="this.parentElement.remove()">&times;</span>';
  chip.dataset.value = val;
  container.insertBefore(chip, input);
  input.value = '';
}

function getTagValues(container) {
  return Array.from(container.querySelectorAll('.tag-chip')).map(function (c) {
    return c.dataset.value;
  });
}

/* ══════════════════════════════════════════════════════════════════════════════
   Grouped upload with drag & drop
   ══════════════════════════════════════════════════════════════════════════════

   Data model
   ----------
   uploadGroups : { [groupId]: { id, name, fileIds: [int] } }
   uploadFileMap: { [fileId]:  { id, file: File, groupId } }

   Each group renders as a card with shared Name / Creators / License controls
   and a chip-area listing its files. Chips are draggable between groups.
   ══════════════════════════════════════════════════════════════════════════════ */

var uploadGroups  = {};   // groupId → group object
var uploadFileMap = {};   // fileId  → file entry
var nextFileId    = 0;
var nextGroupId   = 0;
var dragFileId    = null; // currently-dragged file id

/* ── Open / close upload modal ─────────────────────────────────────────────── */
function openUpload() {
  document.getElementById('uploadTypeLabel').textContent = activeType;
  document.getElementById('upload-files').value = '';
  document.getElementById('uploadGroups').innerHTML = '';
  document.getElementById('uploadStep1').style.display = '';
  document.getElementById('uploadStep2').style.display = 'none';
  var gs = document.getElementById('uploadGlobalStatus');
  gs.textContent = '';
  gs.className = 'upload-status';
  uploadGroups  = {};
  uploadFileMap = {};
  nextFileId    = 0;
  nextGroupId   = 0;
  dragFileId    = null;
  document.getElementById('uploadModal').classList.add('open');
}

function closeUpload() {
  document.getElementById('uploadModal').classList.remove('open');
}

document.getElementById('uploadModal').addEventListener('click', function (e) {
  if (e.target === this) closeUpload();
});

/* ── Map info parser ────────────────────────────────────────────────────────── */
/*
  parseMapInfo reads a Teeworlds/DDNet .map file (ArrayBuffer) and extracts
  the info item (author, version, credits, license).
  Returns { author, version, credits, license } or null if the info item
  is missing or cannot be parsed.
*/
async function parseMapInfo(buffer) {
  var view = new DataView(buffer);
  if (buffer.byteLength < 36) return null;

  var magic = String.fromCharCode(
    view.getUint8(0), view.getUint8(1), view.getUint8(2), view.getUint8(3));
  if (magic !== 'DATA' && magic !== 'ATAD') return null;

  var fileVersion = view.getInt32(4, true);
  if (fileVersion !== 3 && fileVersion !== 4) return null;

  var numItemTypes = view.getInt32(16, true);
  var numItems     = view.getInt32(20, true);
  var numData      = view.getInt32(24, true);
  var sizeItems    = view.getInt32(28, true);
  var sizeData     = view.getInt32(32, true);
  if (numItemTypes < 0 || numItems < 0 || numData < 0 ||
      sizeItems < 0 || sizeData < 0) return null;

  /* Section offsets after the 36-byte header. */
  var off            = 36;
  var itemOffsetsOff = off + numItemTypes * 12;
  var dataOffsetsOff = itemOffsetsOff + numItems * 4;
  var dataSizesEnd   = dataOffsetsOff + numData * 4;
  var itemsBlockOff  = (fileVersion >= 4) ? dataSizesEnd + numData * 4 : dataSizesEnd;
  var dataBlockOff   = itemsBlockOff + sizeItems;

  if (dataBlockOff + sizeData > buffer.byteLength) return null;

  /* Read item offsets. */
  var itemOffsets = [];
  for (var i = 0; i < numItems; i++) {
    itemOffsets.push(view.getInt32(itemOffsetsOff + i * 4, true));
  }

  /* Read data offsets. */
  var dataOffsets = [];
  for (var i = 0; i < numData; i++) {
    dataOffsets.push(view.getInt32(dataOffsetsOff + i * 4, true));
  }

  /* Find info item (typeID=1, id=0) in the items block. */
  var infoData = null;
  for (var i = 0; i < numItems; i++) {
    var itemOff = itemsBlockOff + itemOffsets[i];
    if (itemOff + 8 > buffer.byteLength) continue;
    var typeIDAndID = view.getInt32(itemOff, true);
    var typeID = (typeIDAndID >>> 16) & 0xFFFF;
    var id     = typeIDAndID & 0xFFFF;
    if (typeID === 1 && id === 0) {
      var itemSize = view.getInt32(itemOff + 4, true);
      if (itemSize < 0 || itemSize % 4 !== 0) return null;
      var numInt32s = itemSize / 4;
      if (numInt32s < 5) return null;
      infoData = [];
      for (var j = 0; j < numInt32s; j++) {
        infoData.push(view.getInt32(itemOff + 8 + j * 4, true));
      }
      break;
    }
  }
  if (!infoData) return null;

  /* Decompress a data block and return it as a string. */
  function readDataStr(dataIdx) {
    if (dataIdx < 0 || dataIdx >= numData) return Promise.resolve('');
    var start = dataOffsets[dataIdx];
    var end   = (dataIdx + 1 < numData) ? dataOffsets[dataIdx + 1] : sizeData;
    if (start < 0 || end < start) return Promise.resolve('');

    var chunk = buffer.slice(dataBlockOff + start, dataBlockOff + end);
    if (fileVersion === 3) {
      var bytes = new Uint8Array(chunk);
      var len = bytes.length;
      while (len > 0 && bytes[len - 1] === 0) len--;
      return Promise.resolve(new TextDecoder().decode(bytes.subarray(0, len)));
    }

    /* v4: zlib (RFC 1950) compressed */
    var ds = new DecompressionStream('deflate');
    var writer = ds.writable.getWriter();
    writer.write(new Uint8Array(chunk));
    writer.close();
    return new Response(ds.readable).arrayBuffer().then(function (ab) {
      var out = new Uint8Array(ab);
      var len = out.length;
      while (len > 0 && out[len - 1] === 0) len--;
      return new TextDecoder().decode(out.subarray(0, len));
    }).catch(function () { return ''; });
  }

  /* infoData: [0]=version, [1]=author, [2]=mapversion, [3]=credits, [4]=license */
  return Promise.all([
    readDataStr(infoData[1]),
    readDataStr(infoData[2]),
    readDataStr(infoData[3]),
    readDataStr(infoData[4])
  ]).then(function (strings) {
    var result = { author: strings[0], version: strings[1], credits: strings[2], license: strings[3] };
    console.log('[mapInfo] parsed:', result);
    return result;
  });
}

/* ── Process map files (async with info validation) ────────────────────────── */
async function processMapFiles(files) {
  var gs = document.getElementById('uploadGlobalStatus');
  gs.textContent = 'Reading map files\u2026';
  gs.className = 'upload-status';

  var errors = [];
  var validFiles = [];

  for (var i = 0; i < files.length; i++) {
    var f = files[i];
    try {
      var buf = await f.arrayBuffer();
      var info = await parseMapInfo(buf);
      console.log('[mapInfo] file=%s info=%o', f.name, info);
      if (!info) {
        errors.push(f.name + ': missing map info (author/license metadata)');
        continue;
      }
      validFiles.push({ file: f, info: info });
    } catch (e) {
      errors.push(f.name + ': failed to parse map file');
    }
  }

  if (errors.length) {
    gs.textContent = errors.join('; ');
    gs.className = 'upload-status error';
  } else {
    gs.textContent = '';
    gs.className = 'upload-status';
  }

  if (!validFiles.length) return;

  var nameToGroup = {};
  Object.keys(uploadGroups).forEach(function (gid) {
    var g = uploadGroups[gid];
    nameToGroup[g.name.toLowerCase()] = g.id;
  });

  validFiles.forEach(function (vf) {
    var fid  = nextFileId++;
    var name = nameFromFile(vf.file.name);
    var key  = name.toLowerCase();

    var gid;
    if (nameToGroup[key] !== undefined) {
      gid = nameToGroup[key];
    } else {
      gid = nextGroupId++;
      uploadGroups[gid] = { id: gid, name: name, fileIds: [], mapInfo: vf.info };
      nameToGroup[key] = gid;
    }

    uploadFileMap[fid] = { id: fid, file: vf.file, groupId: gid };
    uploadGroups[gid].fileIds.push(fid);
  });

  renderUploadGroups();
  document.getElementById('uploadStep1').style.display = 'none';
  document.getElementById('uploadStep2').style.display = '';
}

/* ── File selection → auto-group by derived name ───────────────────────────── */
function onFilesSelected(input) {
  var files = Array.from(input.files);
  if (!files.length) return;
  input.value = '';

  /* Maps: parse info field, reject files missing it. */
  if (activeType === 'map') {
    processMapFiles(files);
    return;
  }

  /* Build a map: derivedName → existing groupId (if any). */
  var nameToGroup = {};
  Object.keys(uploadGroups).forEach(function (gid) {
    var g = uploadGroups[gid];
    nameToGroup[g.name.toLowerCase()] = g.id;
  });

  files.forEach(function (f) {
    var fid  = nextFileId++;
    var name = nameFromFile(f.name);
    var key  = name.toLowerCase();

    /* Find or create a group for this name. */
    var gid;
    if (nameToGroup[key] !== undefined) {
      gid = nameToGroup[key];
    } else {
      gid = nextGroupId++;
      uploadGroups[gid] = { id: gid, name: name, fileIds: [] };
      nameToGroup[key] = gid;
    }

    uploadFileMap[fid] = { id: fid, file: f, groupId: gid };
    uploadGroups[gid].fileIds.push(fid);
  });

  renderUploadGroups();
  document.getElementById('uploadStep1').style.display = 'none';
  document.getElementById('uploadStep2').style.display = '';
}

/* ── Render all groups ─────────────────────────────────────────────────────── */
function renderUploadGroups() {
  var container = document.getElementById('uploadGroups');
  container.innerHTML = '';

  Object.keys(uploadGroups).forEach(function (gid) {
    var g = uploadGroups[gid];
    if (!g.fileIds.length) return; // skip empty groups

    var div = document.createElement('div');
    div.className = 'upload-group';
    div.id = 'upload-group-' + g.id;
    div.dataset.groupId = g.id;

    /* Header: shared Name / Creators / License */
    div.innerHTML =
      '<div class="upload-group-header">' +
        '<div class="form-group">' +
          '<label>Name</label>' +
          '<input type="text" class="ui-name" value="' + escapeHtml(g.name) + '" placeholder="Group name" oninput="onGroupNameChange(' + g.id + ', this.value)">' +
        '</div>' +
        '<div class="form-group">' +
          '<label>Creators</label>' +
          '<div class="tag-input ui-creators"><input type="text" placeholder="Type name, press Enter"></div>' +
        '</div>' +
        '<div class="form-group">' +
          '<label>License</label>' +
          '<select class="ui-license">' + licenseOptions + '</select>' +
        '</div>' +
      '</div>' +
      '<div class="upload-group-files" id="upload-group-files-' + g.id + '">' +
        '<input type="file" multiple accept=".png,.map" style="display:none" id="upload-group-add-' + g.id + '" onchange="onGroupFilesSelected(this,' + g.id + ')">' +
        '<button type="button" class="btn-add-variant" title="Add variant" onclick="document.getElementById(\'upload-group-add-' + g.id + '\').click()">&plus;</button>' +
      '</div>' +
      '<div class="upload-group-status" id="upload-group-status-' + g.id + '"></div>';

    container.appendChild(div);
    initTagInput(div.querySelector('.tag-input'));

    /* Pre-fill creators & license from map info (if available). */
    if (g.mapInfo) {
      console.log('[mapInfo] pre-fill group %s: author=%s credits=%s license=%s',
        g.name, JSON.stringify(g.mapInfo.author), JSON.stringify(g.mapInfo.credits), JSON.stringify(g.mapInfo.license));
      var creatorStr = g.mapInfo.author || g.mapInfo.credits || '';
      if (creatorStr) {
        var tagContainer = div.querySelector('.tag-input');
        var tagInput = tagContainer.querySelector('input');
        creatorStr.split(/[,&;]|\band\b|!\s/i).forEach(function (c) {
          c = c.trim();
          if (c) {
            tagInput.value = c;
            commitTag(tagContainer, tagInput);
          }
        });
      }
      if (g.mapInfo.license) {
        var licSel = div.querySelector('.ui-license');
        var matched = matchLicense(g.mapInfo.license);
        console.log('[mapInfo] license raw=%s matched=%s', JSON.stringify(g.mapInfo.license), JSON.stringify(matched));
        licSel.value = matched || 'unknown';
      }
    }

    setupGroupDropZone(div, g.id);
    renderGroupFiles(g.id);
  });
}

/* ── Render file chips inside a group ──────────────────────────────────────── */
function renderGroupFiles(gid) {
  var g = uploadGroups[gid];
  var area = document.getElementById('upload-group-files-' + gid);
  if (!area) return;

  /* Preserve the hidden input + add-button before clearing. */
  var addInput = area.querySelector('input[type="file"]');
  var addBtn   = area.querySelector('.btn-add-variant');
  area.innerHTML = '';

  g.fileIds.forEach(function (fid) {
    var entry = uploadFileMap[fid];
    if (!entry) return;
    var chip = document.createElement('span');
    chip.className = 'upload-file-chip';
    chip.draggable = true;
    chip.dataset.fileId = fid;
    chip.innerHTML =
      '<span class="file-name">' + escapeHtml(entry.file.name) + '</span>' +
      '<span class="chip-remove" title="Remove file">&times;</span>';

    /* Drag start / end */
    chip.addEventListener('dragstart', function (e) {
      dragFileId = fid;
      chip.classList.add('dragging');
      e.dataTransfer.effectAllowed = 'move';
      e.dataTransfer.setData('text/plain', String(fid));
    });
    chip.addEventListener('dragend', function () {
      chip.classList.remove('dragging');
      dragFileId = null;
    });

    /* Remove button */
    chip.querySelector('.chip-remove').addEventListener('click', function () {
      removeFileFromGroup(fid, gid);
    });

    area.appendChild(chip);
  });

  /* Re-append the hidden input + add button so the '+' stays at the end. */
  if (addInput) area.appendChild(addInput);
  if (addBtn)   area.appendChild(addBtn);
}

/* ── Add files to a specific group ─────────────────────────────────────────── */
function onGroupFilesSelected(input, gid) {
  var files = Array.from(input.files);
  input.value = '';
  if (!files.length) return;

  /* Maps: validate info field before accepting. */
  if (activeType === 'map') {
    addMapFilesToGroup(files, gid);
    return;
  }

  var g = uploadGroups[gid];
  if (!g) return;
  files.forEach(function (f) {
    var fid = nextFileId++;
    uploadFileMap[fid] = { id: fid, file: f, groupId: gid };
    g.fileIds.push(fid);
  });
  renderGroupFiles(gid);
}

async function addMapFilesToGroup(files, gid) {
  var g = uploadGroups[gid];
  if (!g) return;
  var errors = [];

  for (var i = 0; i < files.length; i++) {
    var f = files[i];
    try {
      var buf = await f.arrayBuffer();
      var info = await parseMapInfo(buf);
      if (!info) {
        errors.push(f.name);
        continue;
      }
      var fid = nextFileId++;
      uploadFileMap[fid] = { id: fid, file: f, groupId: gid };
      g.fileIds.push(fid);
    } catch (e) {
      errors.push(f.name);
    }
  }

  if (errors.length) {
    var st = document.getElementById('upload-group-status-' + gid);
    if (st) {
      st.textContent = 'Rejected (no map info): ' + errors.join(', ');
      st.className = 'upload-group-status error';
    }
  }
  renderGroupFiles(gid);
}

/* ── Group name live-edit ──────────────────────────────────────────────────── */
function onGroupNameChange(gid, value) {
  if (uploadGroups[gid]) uploadGroups[gid].name = value.trim();
}

/* ── Remove a file from its group ──────────────────────────────────────────── */
function removeFileFromGroup(fid, gid) {
  var g = uploadGroups[gid];
  if (!g) return;
  g.fileIds = g.fileIds.filter(function (id) { return id !== fid; });
  delete uploadFileMap[fid];

  if (g.fileIds.length === 0) {
    delete uploadGroups[gid];
    var el = document.getElementById('upload-group-' + gid);
    if (el) el.remove();

    /* If no groups left, show step 1 again */
    if (Object.keys(uploadGroups).length === 0) {
      document.getElementById('uploadStep1').style.display = '';
      document.getElementById('uploadStep2').style.display = 'none';
    }
  } else {
    renderGroupFiles(gid);
  }
}

/* ── Move a file from one group to another ─────────────────────────────────── */
function moveFileToGroup(fid, targetGid) {
  var entry = uploadFileMap[fid];
  if (!entry) return;
  var srcGid = entry.groupId;
  if (srcGid === targetGid) return;

  /* Remove from source */
  var srcGroup = uploadGroups[srcGid];
  if (srcGroup) {
    srcGroup.fileIds = srcGroup.fileIds.filter(function (id) { return id !== fid; });
    if (srcGroup.fileIds.length === 0) {
      delete uploadGroups[srcGid];
      var el = document.getElementById('upload-group-' + srcGid);
      if (el) el.remove();
    } else {
      renderGroupFiles(srcGid);
    }
  }

  /* Add to target */
  entry.groupId = targetGid;
  var tgtGroup = uploadGroups[targetGid];
  if (tgtGroup) {
    tgtGroup.fileIds.push(fid);
    renderGroupFiles(targetGid);
  }
}

/* ── Drop-zone wiring for a group card ─────────────────────────────────────── */
function setupGroupDropZone(el, gid) {
  var counter = 0; // track nested enter/leave
  el.addEventListener('dragover', function (e) {
    e.preventDefault();
    e.dataTransfer.dropEffect = 'move';
  });
  el.addEventListener('dragenter', function (e) {
    e.preventDefault();
    counter++;
    el.classList.add('drag-over');
  });
  el.addEventListener('dragleave', function () {
    counter--;
    if (counter <= 0) { counter = 0; el.classList.remove('drag-over'); }
  });
  el.addEventListener('drop', function (e) {
    e.preventDefault();
    counter = 0;
    el.classList.remove('drag-over');
    var fid = parseInt(e.dataTransfer.getData('text/plain'), 10);
    if (!isNaN(fid)) moveFileToGroup(fid, gid);
  });
}

/* ══════════════════════════════════════════════════════════════════════════════
   Submit all uploads
   ──────────────────────────────────────────────────────────────────────────── */
function submitAllUploads() {
  var btn = document.getElementById('uploadAllBtn');
  var gs  = document.getElementById('uploadGlobalStatus');
  btn.disabled = true;
  gs.textContent = '';
  gs.className = 'upload-status';

  /* Collect upload jobs: one request per file, sharing group metadata. */
  var jobs  = [];
  var valid = true;

  Object.keys(uploadGroups).forEach(function (gid) {
    var g    = uploadGroups[gid];
    var card = document.getElementById('upload-group-' + gid);
    if (!card) return;

    var name = card.querySelector('.ui-name').value.trim();
    var tagContainer = card.querySelector('.tag-input');
    commitTag(tagContainer, tagContainer.querySelector('input'));
    var creators = getTagValues(tagContainer);
    var license  = card.querySelector('.ui-license').value;
    var st = document.getElementById('upload-group-status-' + gid);

    if (!name || !creators.length) {
      st.textContent = 'Name and at least one creator required.';
      st.className = 'upload-group-status error';
      valid = false;
      return;
    }

    g.fileIds.forEach(function (fid) {
      var entry = uploadFileMap[fid];
      if (!entry) return;
      jobs.push({
        file: entry.file,
        name: name,
        creators: creators,
        license: license,
        groupId: gid
      });
    });
  });

  if (!valid) { btn.disabled = false; return; }
  if (!jobs.length) { btn.disabled = false; return; }

  var done   = 0;
  var failed = 0;
  var total  = jobs.length;

  /* Track per-group progress */
  var groupProgress = {}; // gid → {total, done, failed}
  jobs.forEach(function (job) {
    if (!groupProgress[job.groupId]) {
      groupProgress[job.groupId] = { total: 0, done: 0, failed: 0 };
    }
    groupProgress[job.groupId].total++;
  });

  jobs.forEach(function (job) {
    var st = document.getElementById('upload-group-status-' + job.groupId);
    st.textContent = 'Uploading\u2026';
    st.className = 'upload-group-status pending';

    var metadata = JSON.stringify({ name: job.name, creators: job.creators, license: job.license });
    var fd = new FormData();
    fd.append('metadata', new Blob([metadata], { type: 'application/json' }));
    fd.append('file', job.file);

    fetch('/api/upload/' + encodeURIComponent(activeType), { method: 'POST', body: fd, headers: { 'X-CSRF-Token': getCSRFToken() } })
    .then(function (resp) {
      return resp.json().then(function (body) {
        if (!resp.ok) throw new Error(body.error || 'Upload failed');
        groupProgress[job.groupId].done++;
      });
    })
    .catch(function (err) {
      groupProgress[job.groupId].failed++;
      failed++;
      var gst = document.getElementById('upload-group-status-' + job.groupId);
      if (gst) { gst.textContent = err.message; gst.className = 'upload-group-status error'; }
    })
    .finally(function () {
      done++;
      gs.textContent = done + '/' + total + ' finished' + (failed ? ' (' + failed + ' failed)' : '');
      gs.className = 'upload-status' + (failed ? ' error' : '');

      /* Update per-group status */
      var gp = groupProgress[job.groupId];
      if (gp.done + gp.failed === gp.total) {
        var gst = document.getElementById('upload-group-status-' + job.groupId);
        var card = document.getElementById('upload-group-' + job.groupId);
        if (gp.failed === 0) {
          if (gst) { gst.textContent = gp.done + ' file(s) uploaded'; gst.className = 'upload-group-status success'; }
          if (card) card.classList.add('done');
        }
      }

      if (done === total) {
        htmx.ajax('GET', '/' + activeType, '#content');
        btn.disabled = false;
        if (failed === 0) {
          gs.textContent = 'All uploads successful!';
          gs.className = 'upload-status success';
          setTimeout(closeUpload, 1500);
        }
      }
    });
  });
}

/* ── Admin functions ───────────────────────────────────────────────────────── */

function adminDeleteGroup(assetType, groupID, name) {
  if (!confirm('Delete "' + name + '" and all its variants? This cannot be undone.')) return;
  fetch('/admin/' + assetType + '/' + groupID, { method: 'DELETE', headers: { 'X-CSRF-Token': getCSRFToken() } })
    .then(function (res) {
      if (!res.ok) return res.text().then(function (t) { throw new Error(t); });
      var row = document.querySelector('tr[data-group-id="' + groupID + '"]');
      if (row) row.remove();
      delete selectedGroups[groupID];
      updateSelectionBar();
    })
    .catch(function (err) { alert('Delete failed: ' + err.message); });
}

function adminDeleteSelected() {
  var ids = Object.keys(selectedGroups);
  if (ids.length === 0) return;
  if (!confirm('Delete ' + ids.length + ' selected group(s) and all their variants? This cannot be undone.')) return;
  var tab = document.querySelector('.tab.active');
  var assetType = tab ? tab.getAttribute('data-type') : '';
  var failed = 0;
  var pending = ids.length;
  ids.forEach(function (gid) {
    fetch('/admin/' + assetType + '/' + gid, { method: 'DELETE', headers: { 'X-CSRF-Token': getCSRFToken() } })
      .then(function (res) {
        if (!res.ok) { failed++; return; }
        var row = document.querySelector('tr[data-group-id="' + gid + '"]');
        if (row) row.remove();
        delete selectedGroups[gid];
      })
      .catch(function () { failed++; })
      .finally(function () {
        pending--;
        if (pending === 0) {
          if (failed > 0) alert(failed + ' deletion(s) failed.');
          location.reload();
        }
      });
  });
}

/* ── Edit modal state ──────────────────────────────────────────────────────── */
var editItems = [];      // { item_id, group_value, size, original_filename }
var editPendingFiles = []; // File objects to upload after save
var editTagInput = null;

function openEditModal(assetType, groupID, name, creators, license) {
  document.getElementById('editAssetType').value = assetType;
  document.getElementById('editGroupID').value = groupID;
  document.getElementById('editName').value = name;
  document.getElementById('editTypeLabel').textContent = assetType;
  var status = document.getElementById('editStatus');
  status.textContent = '';
  status.className = 'upload-status';
  editItems = [];
  editPendingFiles = [];

  // Populate creators tag input
  var tagContainer = document.getElementById('editCreatorsTag');
  // Remove old chips
  tagContainer.querySelectorAll('.tag-chip').forEach(function (c) { c.remove(); });
  var tagInput = tagContainer.querySelector('input');
  tagInput.value = '';
  // Add existing creators as chips
  if (creators) {
    creators.split(',').forEach(function (c) {
      c = c.trim();
      if (c) {
        tagInput.value = c;
        commitTag(tagContainer, tagInput);
      }
    });
  }
  // Init tag input handlers (idempotent — re-init is safe since we clone nothing)
  if (!editTagInput) {
    initTagInput(tagContainer);
    editTagInput = true;
  }

  // Populate and pre-select the license dropdown
  var licSel = document.getElementById('editLicense');
  licSel.innerHTML = licenseOptions;
  licSel.value = license || 'unknown';

  // Show modal, then fetch items
  document.getElementById('editModal').classList.add('open');
  document.getElementById('editItemsList').innerHTML = '<span class="upload-status">Loading\u2026</span>';

  fetch('/admin/' + assetType + '/' + groupID + '/items')
    .then(function (res) {
      if (!res.ok) throw new Error('Failed to load items');
      return res.json();
    })
    .then(function (items) {
      editItems = items;
      renderEditItems();
    })
    .catch(function (err) {
      document.getElementById('editItemsList').innerHTML =
        '<span class="upload-status error">' + escapeHtml(err.message) + '</span>';
    });
}

function closeEditModal() {
  document.getElementById('editModal').classList.remove('open');
  editItems = [];
  editPendingFiles = [];
}

document.getElementById('editModal').addEventListener('click', function (e) {
  if (e.target === this) closeEditModal();
});

function renderEditItems() {
  var area = document.getElementById('editItemsList');
  area.innerHTML = '';

  // Existing items (from DB)
  editItems.forEach(function (item) {
    var chip = document.createElement('span');
    chip.className = 'upload-file-chip';
    chip.dataset.itemId = item.item_id;
    var label = item.group_value || item.original_filename;
    var sizeStr = formatFileSize(item.size);
    chip.innerHTML =
      '<span class="file-name">' + escapeHtml(label) + '</span>' +
      '<span class="chip-size">' + sizeStr + '</span>' +
      '<span class="chip-remove" title="Delete variant">&times;</span>';
    chip.querySelector('.chip-remove').addEventListener('click', function () {
      deleteEditItem(item.item_id, chip);
    });
    area.appendChild(chip);
  });

  // Pending new files (not yet uploaded)
  editPendingFiles.forEach(function (entry, idx) {
    var chip = document.createElement('span');
    chip.className = 'upload-file-chip pending';
    chip.dataset.pendingIdx = idx;
    chip.innerHTML =
      '<span class="file-name">' + escapeHtml(entry.name) + '</span>' +
      '<span class="chip-size">new</span>' +
      '<span class="chip-remove" title="Remove">&times;</span>';
    chip.querySelector('.chip-remove').addEventListener('click', function () {
      editPendingFiles.splice(idx, 1);
      renderEditItems();
    });
    area.appendChild(chip);
  });

  if (editItems.length === 0 && editPendingFiles.length === 0) {
    area.innerHTML = '<span class="upload-status">No variants</span>';
  }
}

function deleteEditItem(itemID, chipEl) {
  var assetType = document.getElementById('editAssetType').value;
  var groupID = document.getElementById('editGroupID').value;

  chipEl.classList.add('dragging'); // dim it
  fetch('/admin/' + assetType + '/' + groupID + '/' + itemID, { method: 'DELETE', headers: { 'X-CSRF-Token': getCSRFToken() } })
    .then(function (res) {
      if (!res.ok) return res.text().then(function (t) { throw new Error(t); });
      editItems = editItems.filter(function (it) { return it.item_id !== itemID; });
      renderEditItems();
    })
    .catch(function (err) {
      chipEl.classList.remove('dragging');
      alert('Delete failed: ' + err.message);
    });
}

function onEditFilesSelected(input) {
  var files = Array.from(input.files);
  input.value = '';
  files.forEach(function (f) {
    editPendingFiles.push(f);
  });
  renderEditItems();
}

function submitEdit() {
  var assetType = document.getElementById('editAssetType').value;
  var groupID = document.getElementById('editGroupID').value;
  var name = document.getElementById('editName').value.trim();
  var creators = getTagValues(document.getElementById('editCreatorsTag'));
  var license = document.getElementById('editLicense').value;
  var status = document.getElementById('editStatus');

  if (!name) { status.textContent = 'Name is required'; status.className = 'upload-status error'; return; }

  status.textContent = 'Saving\u2026';
  status.className = 'upload-status';

  // 1. Save metadata (name + creators + license)
  fetch('/admin/' + assetType + '/' + groupID, {
    method: 'PATCH',
    headers: { 'Content-Type': 'application/json', 'X-CSRF-Token': getCSRFToken() },
    body: JSON.stringify({ name: name, creators: creators, license: license })
  })
  .then(function (res) {
    if (!res.ok) return res.text().then(function (t) { throw new Error(t); });
    // 2. Upload pending files (if any)
    if (editPendingFiles.length === 0) return Promise.resolve();
    return uploadPendingFiles(assetType, name, creators, license);
  })
  .then(function () {
    status.textContent = 'Saved!';
    status.className = 'upload-status success';
    setTimeout(function () {
      closeEditModal();
      // Preserve current filters/pagination by re-requesting with the existing query string.
      var qs = window.location.search || '?limit=' + itemsPerPage + '&offset=0';
      htmx.ajax('GET', '/' + activeType + qs, '#content');
    }, 600);
  })
  .catch(function (err) {
    status.textContent = err.message;
    status.className = 'upload-status error';
  });
}

function uploadPendingFiles(assetType, name, creators, license) {
  var chain = Promise.resolve();
  editPendingFiles.forEach(function (file) {
    chain = chain.then(function () {
      var meta = JSON.stringify({ name: name, creators: creators, license: license || 'unknown' });
      var form = new FormData();
      var metaBlob = new Blob([meta], { type: 'application/json' });
      form.append('metadata', metaBlob, 'metadata.json');
      form.append('file', file);
      return fetch('/api/upload/' + encodeURIComponent(assetType), { method: 'POST', body: form, headers: { 'X-CSRF-Token': getCSRFToken() } })
        .then(function (res) {
          if (!res.ok) return res.json().then(function (j) { throw new Error(j.error || 'Upload failed'); });
        });
    });
  });
  return chain;
}

function formatFileSize(bytes) {
  if (bytes >= 1048576) return (bytes / 1048576).toFixed(1) + ' MB';
  if (bytes >= 1024) return (bytes / 1024).toFixed(1) + ' KB';
  return bytes + ' B';
}

/* ── Metadata modal ────────────────────────────────────────────────────────── */

function openMetadataModal(assetType, groupID, name) {
  document.getElementById('metadataGroupName').textContent = name;
  document.getElementById('metadataContent').innerHTML =
    '<span class="upload-status">Loading\u2026</span>';
  document.getElementById('metadataModal').classList.add('open');

  fetch('/admin/' + encodeURIComponent(assetType) + '/' + encodeURIComponent(groupID) + '/metadata')
    .then(function (res) {
      if (!res.ok) throw new Error('Failed to load metadata');
      return res.json();
    })
    .then(function (items) { renderMetadata(items); })
    .catch(function (err) {
      document.getElementById('metadataContent').innerHTML =
        '<span class="upload-status error">' + escapeHtml(err.message) + '</span>';
    });
}

function closeMetadataModal() {
  document.getElementById('metadataModal').classList.remove('open');
}

document.getElementById('metadataModal').addEventListener('click', function (e) {
  if (e.target === this) closeMetadataModal();
});

function renderMetadata(items) {
  var area = document.getElementById('metadataContent');
  if (!items.length) {
    area.innerHTML = '<span class="upload-status">No items found.</span>';
    return;
  }
  var html = '';
  items.forEach(function (item) {
    var label = item.group_value || item.original_filename || item.item_id;
    html += '<div class="metadata-card">';
    html += '<div class="metadata-card-header">' + escapeHtml(label) +
      ' <span class="chip-size">' + formatFileSize(item.size) + '</span></div>';
    html += '<table class="metadata-table">';
    html += metaRow('Item ID', item.item_id);
    html += metaRow('Original file', item.original_filename);
    html += metaRow('Created at', item.created_at);
    html += metaRow('IP', item.creator_ip);
    html += metaRow('User-Agent', item.creator_agent);
    html += metaRow('Accept-Language', item.accept_language);
    html += metaRow('Referer', item.referer);
    html += metaRow('Content-Type', item.content_type);
    html += metaRow('Request ID', item.request_id);
    html += '</table></div>';
  });
  area.innerHTML = html;
}

function metaRow(label, value) {
  var display = value ? escapeHtml(value) : '<span class="text-dim">\u2014</span>';
  return '<tr><td class="metadata-label">' + escapeHtml(label) + '</td><td class="metadata-value">' + display + '</td></tr>';
}

/* ══════════════════════════════════════════════════════════════════════════════
   Multi-select & bulk download
   ══════════════════════════════════════════════════════════════════════════════ */

var selectedGroups = {};  // groupId → true

function toggleRowSelect(cb) {
  var gid = cb.getAttribute('data-group-id');
  if (cb.checked) {
    selectedGroups[gid] = true;
  } else {
    delete selectedGroups[gid];
  }
  cb.closest('tr').classList.toggle('row-selected', cb.checked);
  updateSelectionBar();
}

function toggleSelectAll(cb) {
  var boxes = document.querySelectorAll('.row-select');
  boxes.forEach(function (box) {
    box.checked = cb.checked;
    var gid = box.getAttribute('data-group-id');
    if (cb.checked) {
      selectedGroups[gid] = true;
    } else {
      delete selectedGroups[gid];
    }
    box.closest('tr').classList.toggle('row-selected', cb.checked);
  });
  updateSelectionBar();
}

function clearSelection() {
  selectedGroups = {};
  var boxes = document.querySelectorAll('.row-select');
  boxes.forEach(function (box) {
    box.checked = false;
    box.closest('tr').classList.remove('row-selected');
  });
  var sa = document.getElementById('selectAll');
  if (sa) sa.checked = false;
  updateSelectionBar();
}

function updateSelectionBar() {
  var ids = Object.keys(selectedGroups);
  var bar = document.getElementById('selectionBar');
  if (!bar) return;
  if (ids.length === 0) {
    bar.style.display = 'none';
    return;
  }
  bar.style.display = '';
  document.getElementById('selectionCount').textContent = ids.length + ' selected';
}

function downloadSelected() {
  var ids = Object.keys(selectedGroups);
  if (ids.length === 0) return;
  var body = JSON.stringify({ group_ids: ids });
  fetch('/api/download/bundle', {
    method: 'POST',
    headers: {
      'Content-Type': 'application/json',
      'X-CSRF-Token': getCSRFToken()
    },
    body: body
  }).then(function (resp) {
    if (!resp.ok) {
      return resp.json().then(function (e) { alert(e.error || 'Download failed'); });
    }
    return resp.blob().then(function (blob) {
      var url = URL.createObjectURL(blob);
      var a = document.createElement('a');
      a.href = url;
      a.download = 'assets.zip';
      document.body.appendChild(a);
      a.click();
      a.remove();
      URL.revokeObjectURL(url);
    });
  });
}

/* Restore checkbox state after HTMX content swaps */
document.addEventListener('htmx:afterSwap', function (e) {
  if (e.detail.target.id !== 'content') return;
  var ids = Object.keys(selectedGroups);
  if (ids.length === 0) return;
  ids.forEach(function (gid) {
    var cb = document.querySelector('.row-select[data-group-id="' + gid + '"]');
    if (cb) {
      cb.checked = true;
      cb.closest('tr').classList.add('row-selected');
    }
  });
  updateSelectionBar();
});
