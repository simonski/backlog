const connection = document.getElementById("connection");
const build = document.querySelector('meta[name="backlog-build"]')?.content;
const reducedMotion = window.matchMedia("(prefers-reduced-motion: reduce)");
const archiveDialog = document.getElementById("archive-confirmation");

document.addEventListener("keydown", event => {
  if (event.key === "Tab") event.preventDefault();
});

function archiveTargetSelector(target) {
  if (target.id) return `#${CSS.escape(target.id)}`;
  if (target.dataset.projectCard) return `[data-project-card="${CSS.escape(target.dataset.projectCard)}"]`;
  return `[data-archive-ticket="${CSS.escape(target.dataset.archiveTicket)}"]`;
}

if (archiveDialog) {
  const archiveMenu = document.getElementById("archive-menu");
  const archiveItem = archiveMenu.querySelector("[role=menuitem]");
  const archiveRequest = document.getElementById("archive-request");
  const colorOptions = document.getElementById("project-color-options");
  const colorButtons = [...colorOptions.querySelectorAll("[data-project-color]")];
  const colorRequest = document.getElementById("project-color-request");
  let menuTargetSelector;

  function confirmArchive(target) {
    const { archiveKind, archiveName, archiveAction, archiveTicket } = target.dataset;
    const details = {
      project: ["Archive project?", `Hide “${archiveName}” and its tickets from the dashboard?`],
      ticket: ["Archive ticket?", `Hide “${archiveName}” from the dashboard?`],
      column: [`Archive all ${archiveName} tickets?`, "Hide the tickets in this column from the dashboard?"],
    };
    const [title, message] = details[archiveKind];
    archiveRequest.action = archiveAction;
    archiveRequest.elements.archived.disabled = archiveKind !== "project";
    archiveRequest.elements.ticket_key.disabled = archiveKind !== "ticket";
    archiveRequest.elements.ticket_key.value = archiveTicket || "";
    archiveDialog.querySelector("h2").textContent = title;
    archiveDialog.querySelector("p").textContent = message;
    archiveDialog.returnValue = "";
    archiveDialog.showModal();
  }

  function showArchiveMenu(target, x, y) {
    menuTargetSelector = archiveTargetSelector(target);
    colorOptions.hidden = !target.dataset.colorAction;
    archiveMenu.setAttribute("aria-label", colorOptions.hidden ? "Archive actions" : "Project actions");
    for (const button of colorButtons) {
      button.setAttribute("aria-checked", String(button.dataset.projectColor === target.dataset.colorPreset));
    }
    archiveItem.querySelector("span").textContent = target.dataset.archiveKind === "column" ? "Archive all" : "Archive";
    archiveItem.setAttribute("aria-disabled", String(target.hasAttribute("data-archive-empty")));
    archiveMenu.hidden = false;
    archiveMenu.style.left = `${Math.max(4, Math.min(x, innerWidth - archiveMenu.offsetWidth - 4))}px`;
    archiveMenu.style.top = `${Math.max(4, Math.min(y, innerHeight - archiveMenu.offsetHeight - 4))}px`;
    archiveItem.focus({ preventScroll: true });
  }

  function closeArchiveMenu(restoreFocus) {
    if (archiveMenu.hidden) return;
    archiveMenu.hidden = true;
    if (restoreFocus) {
      const target = document.querySelector(menuTargetSelector);
      (target?.querySelector(".project-card-link") || target)?.focus({ preventScroll: true });
    }
  }

  document.addEventListener("click", event => {
    const button = event.target.closest("[data-archive-confirm]");
    if (button) confirmArchive(button.closest("[data-archive-action]"));
  });
  document.addEventListener("contextmenu", event => {
    const target = event.target.closest("[data-archive-menu]");
    if (!target || archiveDialog.open) return;
    event.preventDefault();
    showArchiveMenu(target, event.clientX, event.clientY);
  });
  document.addEventListener("keydown", event => {
    if (!archiveMenu.hidden && event.key === "Escape") {
      event.preventDefault();
      closeArchiveMenu(true);
      return;
    }
    if (!archiveMenu.hidden && ["ArrowUp", "ArrowDown", "ArrowLeft", "ArrowRight"].includes(event.key)) {
      event.preventDefault();
      const items = colorOptions.hidden ? [archiveItem] : [archiveItem, ...colorButtons];
      const current = items.indexOf(document.activeElement);
      const step = event.key === "ArrowUp" || event.key === "ArrowLeft" ? -1 : 1;
      items[(current + step + items.length) % items.length].focus();
      return;
    }
    const target = event.target.closest("[data-archive-menu]");
    if (!target || (event.key !== "ContextMenu" && !(event.shiftKey && event.key === "F10"))) return;
    event.preventDefault();
    const anchor = (target.querySelector(".task-group-heading, .project-card-link") || target).getBoundingClientRect();
    showArchiveMenu(target, anchor.left, anchor.bottom);
  });
  document.addEventListener("pointerdown", event => {
    if (!archiveMenu.contains(event.target)) closeArchiveMenu(false);
  });
  archiveItem.addEventListener("click", () => {
    const target = document.querySelector(menuTargetSelector);
    if (!target || target.hasAttribute("data-archive-empty") || archiveItem.getAttribute("aria-disabled") === "true") return;
    closeArchiveMenu(true);
    confirmArchive(target);
  });
  colorOptions.addEventListener("click", event => {
    const button = event.target.closest("[data-project-color]");
    const target = document.querySelector(menuTargetSelector);
    if (!button || !target?.dataset.colorAction) return;
    colorRequest.action = target.dataset.colorAction;
    colorRequest.elements.preset.value = button.dataset.projectColor;
    colorRequest.requestSubmit();
  });
  archiveDialog.addEventListener("close", () => {
    if (archiveDialog.returnValue === "archive") archiveRequest.requestSubmit();
  });
}

function updateActiveTimers(root, now) {
  for (const timer of root.querySelectorAll("[data-started-at]")) {
    const elapsed = Number(timer.dataset.activeMs) + Math.max(0, now - Number(timer.dataset.startedAt));
    const seconds = Math.floor(elapsed / 1000);
    const value = [Math.floor(seconds / 3600), Math.floor(seconds / 60) % 60, seconds % 60]
      .map(part => String(part).padStart(2, "0")).join(":");
    if (timer.textContent !== value) timer.textContent = value;
  }
}

function projectCardPositions(region) {
  const list = region.querySelector(".project-list");
  if (!list) return new Map();
  const listTop = list.getBoundingClientRect().top;
  return new Map(Array.from(list.children, (card, rank) => [
    card.dataset.projectCard, { card, rank, top: card.getBoundingClientRect().top - listTop },
  ]));
}

function animateProjectOrder(region, previous) {
  for (const [id, position] of projectCardPositions(region)) {
    const old = previous.get(id);
    if (!old || old.rank === position.rank) continue;
    const offset = old.top - position.top;
    if (Math.abs(offset) < 1) continue;
    const zIndex = offset > 0 ? 1 : 0;
    position.card.animate(
      [{ transform: `translateY(${offset}px)`, zIndex }, { transform: "translateY(0)", zIndex }],
      { duration: 420, easing: "cubic-bezier(.2, .75, .25, 1)" },
    );
  }
}

function refreshRegion(incoming, id) {
  const current = document.getElementById(id);
  const next = incoming.getElementById(id);
  if (!next || current.innerHTML === next.innerHTML) return;

  const active = document.activeElement;
  let focusSelector;
  if (current.contains(active)) {
    if (active.matches("a[href]")) {
      focusSelector = `a[href="${CSS.escape(active.getAttribute("href"))}"]`;
    } else if (active.matches("[data-archive-confirm], [data-archive-menu]")) {
      focusSelector = archiveTargetSelector(active);
    } else if (active.matches(".activity-tools button")) {
      focusSelector = `#${CSS.escape(active.id)}`;
    }
  }
  let previousTasks;
  let previousProjects;
  if (id === "content" && !reducedMotion.matches) {
    previousProjects = projectCardPositions(current);
    previousTasks = new Map(Array.from(
      current.querySelectorAll("[data-task-id]"),
      (task) => [task.dataset.taskId, task.outerHTML],
    ));
  }

  current.replaceChildren(...next.childNodes);
  if (focusSelector) {
    const replacement = current.querySelector(focusSelector) || document.getElementById("content");
    replacement.focus({ preventScroll: true });
  }
  if (previousProjects?.size) animateProjectOrder(current, previousProjects);
  if (previousTasks) {
    for (const task of current.querySelectorAll("[data-task-id]")) {
      if (previousTasks.get(task.dataset.taskId) === task.outerHTML) continue;
      task.animate(
        [{ opacity: 0.55, transform: "translateY(5px)" }, { opacity: 1, transform: "none" }],
        { duration: 320, easing: "cubic-bezier(.2, .75, .25, 1)" },
      );
    }
  }
}

async function refresh() {
  try {
    const response = await fetch(window.location.href, {
      cache: "no-store",
      signal: AbortSignal.timeout(2000),
    });
    if (response.status === 404 && window.location.pathname.startsWith("/projects/")) {
      window.location.replace("/");
      return;
    }
    if (!response.ok) throw new Error("Dashboard unavailable");
    const incoming = new DOMParser().parseFromString(await response.text(), "text/html");
    const incomingBuild = incoming.querySelector('meta[name="backlog-build"]')?.content;
    if (incomingBuild && incomingBuild !== build) {
      window.location.reload();
      return;
    }
    // Compare both renders at the same time so ticking alone never replaces cards.
    const now = Date.now();
    updateActiveTimers(document, now);
    updateActiveTimers(incoming, now);
    refreshRegion(incoming, "navigation");
    refreshRegion(incoming, "content");
    document.title = incoming.title;
    connection.hidden = true;
    connection.textContent = "";
  } catch {
    if (connection.hidden) {
      connection.hidden = false;
      connection.textContent = "Disconnected";
    }
  }
  window.setTimeout(refresh, 1500);
}

if (document.querySelector(".task-groups")) {
  updateActiveTimers(document, Date.now());
  window.setInterval(() => updateActiveTimers(document, Date.now()), 250);
}
if (connection) window.setTimeout(refresh, 1500);
