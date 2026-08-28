const docEl = document.getElementById('doc');
const listEl = document.getElementById('comment-list');
const btnEl = document.getElementById('comment-btn');
const formEl = document.getElementById('comment-form');
const emptyStateEl = document.getElementById('empty-state');
const toastEl = document.getElementById('toast');

let comments = [];
let pending = null; // captured selection: {startLine, endLine, quote}
let editingId = null; // id of the comment currently shown with an inline edit form
let animatedOnce = false; // sidebar cards play their entrance animation once
let toastTimer = null;

init();

async function init() {
  renderMermaid();
  try {
    comments = await api('GET', '/api/comments');
  } catch (err) {
    showToast('Request failed — try again.');
  }
  renderList();
}

function renderMermaid() {
  document.querySelectorAll('#doc code.language-mermaid').forEach((code) => {
    const holder = document.createElement('pre');
    holder.className = 'mermaid';
    holder.textContent = code.textContent;
    code.closest('pre').replaceWith(holder);
  });
  if (window.mermaid) {
    mermaid.initialize({ startOnLoad: false });
    const result = mermaid.run();
    // Diagram rendering shifts block heights, so re-layout once it settles.
    if (result && typeof result.then === 'function') {
      result.then(() => layoutComments());
    }
  }
}

window.addEventListener('resize', () => layoutComments());

function reducedMotion() {
  return window.matchMedia('(prefers-reduced-motion: reduce)').matches;
}

function isNarrow() {
  return window.matchMedia('(max-width: 900px)').matches;
}

function isSaveShortcut(e) {
  return (e.metaKey || e.ctrlKey) && e.key === 'Enter';
}

function showToast(msg) {
  toastEl.textContent = msg;
  toastEl.hidden = false;
  clearTimeout(toastTimer);
  toastTimer = setTimeout(() => { toastEl.hidden = true; }, 3000);
}

async function api(method, url, body) {
  const res = await fetch(url, {
    method,
    headers: body ? { 'Content-Type': 'application/json' } : undefined,
    body: body ? JSON.stringify(body) : undefined,
  });
  if (!res.ok) throw new Error(`${method} ${url}: ${res.status}`);
  return res.status === 204 ? null : res.json();
}

// --- text selection -> floating comment button ---

let selectionRect = null; // bounding rect of the range behind `pending`, used to place the form

document.addEventListener('mouseup', (e) => {
  if (btnEl.contains(e.target) || formEl.contains(e.target)) return;
  setTimeout(updateButton, 0); // let the browser finalize the selection
});

// A double-click's finalized selection can arrive after mouseup already
// ran, so re-check on dblclick too.
document.addEventListener('dblclick', (e) => {
  if (btnEl.contains(e.target) || formEl.contains(e.target)) return;
  setTimeout(updateButton, 0);
});

function updateButton() {
  const sel = window.getSelection();
  const text = sel && !sel.isCollapsed ? sel.toString().trim() : '';
  if (!text) {
    btnEl.hidden = true;
    return;
  }
  const range = sel.getRangeAt(0);
  const blocks = intersectingBlocks(range);
  if (blocks.length === 0) {
    btnEl.hidden = true;
    return;
  }
  pending = {
    startLine: linesOf(blocks[0])[0],
    endLine: linesOf(blocks[blocks.length - 1])[1],
    quote: text,
  };
  selectionRect = range.getBoundingClientRect();
  positionFloating(btnEl, selectionRect);
}

// The #doc > .block elements a selection range actually overlaps, in
// document order. Ancestor lookup alone (closest('#doc > .block')) isn't
// reliable here: a triple-click (line/paragraph selection) commonly makes
// the browser set the range's end boundary just inside the FOLLOWING
// block (e.g. at offset 0 of its first element), with none of that
// block's own content selected. intersectsNode() reports that block as
// touched too, so each candidate is double-checked by clamping the
// selection to the block's own bounds and requiring real text there.
function intersectingBlocks(range) {
  const blocks = [...docEl.querySelectorAll(':scope > .block')];
  return blocks.filter((block) => range.intersectsNode(block) && blockSelectionText(range, block) !== '');
}

// The portion of `range` that falls within `block`, clamped to its bounds.
function blockSelectionText(range, block) {
  const sub = range.cloneRange();
  const blockRange = document.createRange();
  blockRange.selectNodeContents(block);
  if (range.compareBoundaryPoints(Range.START_TO_START, blockRange) < 0) {
    sub.setStart(block, 0);
  }
  if (range.compareBoundaryPoints(Range.END_TO_END, blockRange) > 0) {
    sub.setEnd(block, block.childNodes.length);
  }
  return sub.toString().trim();
}

function linesOf(block) {
  return block.dataset.lines.split('-').map(Number);
}

function formatLines(startLine, endLine) {
  return endLine > startLine ? `L${startLine}-${endLine}` : `L${startLine}`;
}

// Position a floating element (the comment button or form) below a
// selection's bounding rect, clamped to stay fully inside the viewport:
// flip above the selection when there's no room below, and pin
// horizontally so it never overflows either edge.
function positionFloating(el, rect) {
  el.hidden = false; // must be visible (and laid out) to measure its size
  const margin = 8;
  const w = el.offsetWidth;
  const h = el.offsetHeight;
  const minLeft = window.scrollX + margin;
  const maxLeft = window.scrollX + window.innerWidth - w - margin;
  let left = Math.min(Math.max(window.scrollX + rect.left, minLeft), maxLeft);
  let top = window.scrollY + rect.bottom + 6;
  if (top + h > window.scrollY + window.innerHeight - margin) {
    top = window.scrollY + rect.top - h - 6; // flip above the selection
  }
  top = Math.max(top, window.scrollY + margin);
  el.style.left = `${left}px`;
  el.style.top = `${top}px`;
}

btnEl.addEventListener('mousedown', (e) => {
  e.preventDefault(); // keep the selection alive
  openForm();
});

// --- comment form ---

function openForm() {
  btnEl.hidden = true;
  formEl.querySelector('.form-quote-label').textContent = formatLines(pending.startLine, pending.endLine);
  formEl.querySelector('.form-quote-text').textContent = pending.quote;
  const ta = formEl.querySelector('textarea');
  ta.value = '';
  positionFloating(formEl, selectionRect);
  ta.focus();
}

const saveBtn = formEl.querySelector('.save');
saveBtn.addEventListener('click', async () => {
  if (saveBtn.disabled) return; // ignore re-entrant clicks while a save is in flight
  const body = formEl.querySelector('textarea').value.trim();
  if (!body) return;
  saveBtn.disabled = true;
  try {
    const c = await api('POST', '/api/comments', { ...pending, body });
    comments.push(c);
    formEl.hidden = true;
    renderList();
  } catch (err) {
    showToast('Request failed — try again.');
  } finally {
    saveBtn.disabled = false;
  }
});

formEl.querySelector('textarea').addEventListener('keydown', (e) => {
  if (isSaveShortcut(e)) {
    e.preventDefault();
    saveBtn.click();
  }
});

formEl.querySelector('.cancel').addEventListener('click', () => {
  formEl.hidden = true;
});

// --- sidebar ---

function sortedComments() {
  return [...comments].sort((a, b) => a.startLine - b.startLine);
}

// Anchor a comment to the block whose line range contains its start line,
// falling back to the nearest preceding block, then the first block.
function findAnchorBlock(startLine) {
  const blocks = [...docEl.querySelectorAll(':scope > .block')];
  if (blocks.length === 0) return null;
  for (const b of blocks) {
    const [a, z] = linesOf(b);
    if (startLine >= a && startLine <= z) return b;
  }
  let fallback = null;
  for (const b of blocks) {
    if (linesOf(b)[0] <= startLine) fallback = b;
  }
  return fallback || blocks[0];
}

// Keep the persistent marker tint in sync with which blocks currently have
// at least one comment anchored to them.
function updateCommentedBlocks() {
  docEl.querySelectorAll(':scope > .block.commented').forEach((b) => b.classList.remove('commented'));
  for (const c of comments) {
    const anchor = findAnchorBlock(c.startLine);
    if (anchor) anchor.classList.add('commented');
  }
}

// Position each sidebar card beside its anchor block (GitHub-PR style),
// resolving vertical collisions by pushing later cards further down.
function layoutComments() {
  const cards = [...listEl.children];
  if (isNarrow()) {
    // Below the single-column breakpoint, cards stack in normal flow.
    cards.forEach((li) => { li.style.top = ''; });
    listEl.style.height = '';
    return;
  }
  if (cards.length === 0) {
    listEl.style.height = '0px';
    return;
  }
  const items = sortedComments();
  const listTop = listEl.getBoundingClientRect().top + window.scrollY;
  let prevBottom = -Infinity;
  cards.forEach((li, i) => {
    const anchor = findAnchorBlock(items[i].startLine);
    const desiredTop = anchor
      ? anchor.getBoundingClientRect().top + window.scrollY - listTop
      : 0;
    const top = Math.max(desiredTop, prevBottom + 8);
    li.style.top = `${top}px`;
    prevBottom = top + li.offsetHeight;
  });
  listEl.style.height = `${prevBottom}px`;
  if (!animatedOnce) {
    animatedOnce = true;
    cards.forEach((li) => li.classList.add('enter'));
    requestAnimationFrame(() => {
      requestAnimationFrame(() => cards.forEach((li) => li.classList.add('enter-active')));
    });
  }
}

function renderList() {
  const n = comments.length;
  document.getElementById('comment-count').textContent =
    n === 1 ? '1 comment' : `${n} comments`;
  emptyStateEl.hidden = comments.length > 0;
  // Cards are about to be destroyed and rebuilt, so a card the mouse is
  // currently over won't get its mouseleave event; clear any highlight
  // left over from that before rebuilding.
  docEl.querySelectorAll(':scope > .block.highlight').forEach((b) => b.classList.remove('highlight'));
  listEl.replaceChildren();
  for (const c of sortedComments()) {
    const li = document.createElement('li');
    const editing = c.id === editingId;
    li.innerHTML = editing
      ? `
      <div class="card-top">
        <span class="lines"></span>
        <div class="actions">
          <button class="save-edit">Save</button>
          <button class="cancel-edit">Cancel</button>
        </div>
      </div>
      <blockquote><span class="quote-text"></span></blockquote>
      <textarea class="edit-body"></textarea>`
      : `
      <div class="card-top">
        <span class="lines"></span>
        <div class="actions">
          <button class="edit">Edit</button>
          <button class="del">Delete</button>
        </div>
      </div>
      <blockquote><span class="quote-text"></span></blockquote>
      <p class="body"></p>`;
    li.querySelector('.lines').textContent = formatLines(c.startLine, c.endLine);
    li.querySelector('.quote-text').textContent = c.quote;

    const anchor = findAnchorBlock(c.startLine);
    if (anchor) {
      li.addEventListener('mouseenter', () => anchor.classList.add('highlight'));
      li.addEventListener('mouseleave', () => anchor.classList.remove('highlight'));
    }
    li.addEventListener('click', (e) => {
      if (e.target.closest('button, textarea')) return;
      const sel = window.getSelection();
      if (sel && !sel.isCollapsed) return; // don't hijack a text selection
      if (anchor) anchor.scrollIntoView({ behavior: reducedMotion() ? 'auto' : 'smooth', block: 'center' });
    });

    if (editing) {
      const ta = li.querySelector('.edit-body');
      ta.value = c.body;
      const saveEditBtn = li.querySelector('.save-edit');
      ta.addEventListener('keydown', (e) => {
        if (isSaveShortcut(e)) {
          e.preventDefault();
          saveEditBtn.click();
        }
      });
      saveEditBtn.addEventListener('click', async () => {
        if (saveEditBtn.disabled) return; // ignore re-entrant clicks while a save is in flight
        const body = ta.value.trim();
        if (!body) return;
        saveEditBtn.disabled = true;
        try {
          const updated = await api('PUT', `/api/comments/${c.id}`, { body });
          c.body = updated.body;
          editingId = null;
          renderList();
        } catch (err) {
          showToast('Request failed — try again.');
        } finally {
          saveEditBtn.disabled = false;
        }
      });
      li.querySelector('.cancel-edit').addEventListener('click', () => {
        editingId = null;
        renderList();
      });
      listEl.appendChild(li);
      ta.focus();
      continue;
    }

    li.querySelector('.body').textContent = c.body;
    li.querySelector('.edit').addEventListener('click', () => {
      editingId = c.id;
      renderList();
    });
    li.querySelector('.del').addEventListener('click', async () => {
      try {
        await api('DELETE', `/api/comments/${c.id}`);
        comments = comments.filter((x) => x.id !== c.id);
        renderList();
      } catch (err) {
        showToast('Request failed — try again.');
      }
    });
    listEl.appendChild(li);
  }
  updateCommentedBlocks();
  layoutComments();
}

// --- submit ---

// Grows the overall-comment field once it has content, so a reviewer who
// clicked away doesn't have the field collapse back over their draft.
const overallEl = document.getElementById('overall');
overallEl.addEventListener('input', () => {
  overallEl.classList.toggle('grown', overallEl.value.trim().length > 0);
});

for (const [id, verdict] of [
  ['approve', 'APPROVE'],
  ['request-changes', 'REQUEST_CHANGES'],
]) {
  document.getElementById(id).addEventListener('click', async () => {
    try {
      await api('POST', '/api/submit', {
        verdict,
        overall: document.getElementById('overall').value,
      });
      document.body.innerHTML =
        '<main class="done"><p class="eyebrow">Review submitted</p>' +
        '<h1>Feedback sent to the terminal</h1>' +
        '<p class="muted">You can close this tab.</p></main>';
    } catch (err) {
      showToast('Request failed — try again.');
    }
  });
}
