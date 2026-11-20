const connection = document.getElementById("connection");
const build = document.querySelector('meta[name="backlog-build"]')?.content;
const reducedMotion = window.matchMedia("(prefers-reduced-motion: reduce)");

function refreshRegion(incoming, id) {
  const current = document.getElementById(id);
  const next = incoming.getElementById(id);
  if (!next || current.innerHTML === next.innerHTML) return;

  const active = document.activeElement;
  let focusSelector;
  if (current.contains(active)) {
    if (active.matches("a[href]")) {
      focusSelector = `a[href="${CSS.escape(active.getAttribute("href"))}"]`;
    } else if (active.matches('button[name="archived"]')) {
      focusSelector = 'button[name="archived"]';
    }
  }
  let previousTasks;
  if (id === "content" && !reducedMotion.matches) {
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
    if (!response.ok) throw new Error("Dashboard unavailable");
    const incoming = new DOMParser().parseFromString(await response.text(), "text/html");
    const incomingBuild = incoming.querySelector('meta[name="backlog-build"]')?.content;
    if (incomingBuild && incomingBuild !== build) {
      window.location.reload();
      return;
    }
    refreshRegion(incoming, "navigation");
    refreshRegion(incoming, "content");
    document.title = incoming.title;
    if (connection.textContent !== "Live") connection.textContent = "Live";
    connection.classList.remove("offline");
  } catch {
    if (connection.textContent !== "Disconnected") connection.textContent = "Disconnected";
    connection.classList.add("offline");
  }
  window.setTimeout(refresh, 1500);
}

window.setTimeout(refresh, 1500);
