/* ══════════════════════════════════════════════════════════════════════════════
   Teeworlds Assets – Upload & UI logic
   ══════════════════════════════════════════════════════════════════════════════ */

/* global activeType is set by the template inline script */

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
  if (e.key === 'Escape') closePreview();

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
}

/* ── Column sorting ────────────────────────────────────────────────────────── */
/*
  currentSort is an ordered array of { field, dir } objects.
  - Without Shift: clicking a column replaces the entire sort with that column.
    Clicking the same column toggles asc → desc → unsorted.
  - With Shift: clicking adds/toggles the column as a secondary sort (max 2).
*/
var currentSort = [{field: 'name', dir: 'asc'}];

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
  var url = '/' + activeType + '?limit=20&offset=0';
  var search = document.getElementById('search');
  var q = search ? search.value.trim() : '';
  if (q) url += '&q=' + encodeURIComponent(q);
  if (sortParam) url += '&sort=' + encodeURIComponent(sortParam);
  htmx.ajax('GET', url, '#content');
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
  '<option value="custom">Custom / Other</option>' +
  '<option value="unknown">Unknown</option>';

function nameFromFile(filename) {
  return filename.replace(/\.[^/.]+$/, '').replace(/[_-]+/g, ' ').trim();
}

function escapeHtml(s) {
  var d = document.createElement('div');
  d.textContent = s;
  return d.innerHTML;
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

/* ── File selection → auto-group by derived name ───────────────────────────── */
function onFilesSelected(input) {
  var files = Array.from(input.files);
  if (!files.length) return;
  input.value = '';

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
      '<div class="upload-group-files" id="upload-group-files-' + g.id + '"></div>' +
      '<div class="upload-group-status" id="upload-group-status-' + g.id + '"></div>';

    container.appendChild(div);
    initTagInput(div.querySelector('.tag-input'));
    setupGroupDropZone(div, g.id);
    renderGroupFiles(g.id);
  });
}

/* ── Render file chips inside a group ──────────────────────────────────────── */
function renderGroupFiles(gid) {
  var g = uploadGroups[gid];
  var area = document.getElementById('upload-group-files-' + gid);
  if (!area) return;
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

    fetch('/api/upload/' + encodeURIComponent(activeType), { method: 'POST', body: fd })
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
