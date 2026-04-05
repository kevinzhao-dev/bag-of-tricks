(function () {
  "use strict";

  const grid = document.getElementById("grid");
  const status = document.getElementById("status");
  const helpEl = document.getElementById("help");
  const gotoDialog = document.getElementById("goto-dialog");
  const gotoInput = document.getElementById("goto-input");

  let state = null;
  let gotoMode = false;

  // --- SSE connection ---
  function connect() {
    const es = new EventSource("/events");
    es.onmessage = function (e) {
      state = JSON.parse(e.data);
      render();
    };
    es.onerror = function () {
      // EventSource will auto-reconnect
    };
  }

  // --- Send command to server ---
  function send(cmd) {
    fetch("/cmd", {
      method: "POST",
      headers: { "Content-Type": "application/json" },
      body: JSON.stringify(cmd),
    });
  }

  // --- Preload adjacent images ---
  const preloadCache = {};
  function preloadAdjacent() {
    if (!state) return;
    state.panes.forEach(function (pane) {
      const total = state.total_images;
      [-1, 1, -2, 2].forEach(function (d) {
        const idx = pane.image_index + d;
        if (idx >= 0 && idx < total && !preloadCache[idx]) {
          const img = new Image();
          img.src = "/image?idx=" + idx;
          preloadCache[idx] = img;
        }
      });
    });
  }

  // --- Render the grid ---
  function render() {
    if (!state) return;

    // Update grid layout
    grid.style.gridTemplateColumns = "repeat(" + state.grid_cols + ", 1fr)";
    grid.style.gridTemplateRows = "repeat(" + state.grid_rows + ", 1fr)";

    // Build panes
    // Reuse existing DOM nodes if possible
    while (grid.children.length > state.panes.length) {
      grid.removeChild(grid.lastChild);
    }

    state.panes.forEach(function (pane, i) {
      let el = grid.children[i];
      if (!el) {
        el = document.createElement("div");
        el.className = "pane";
        el.innerHTML =
          '<span class="pane-number"></span>' +
          '<img alt="" />' +
          '<div class="pane-info"><span class="info-name"></span><span class="info-date"></span><span class="info-idx"></span></div>';
        el.addEventListener("click", function () {
          send({ cmd: "focus", pane: i });
        });
        grid.appendChild(el);
      }

      el.className = pane.focused ? "pane focused" : "pane";
      el.querySelector(".pane-number").textContent = i + 1;

      const img = el.querySelector("img");
      const newSrc = pane.image_url;
      if (img.getAttribute("src") !== newSrc) {
        img.src = newSrc;
      }

      el.querySelector(".info-name").textContent = pane.image_name;
      el.querySelector(".info-date").textContent = pane.image_date;
      if (state.speed_mode) {
        var pos = pane.image_index - pane.chunk_start + 1;
        var total = pane.chunk_end - pane.chunk_start;
        el.querySelector(".info-idx").textContent =
          pos + "/" + total + (pane.chunk_done ? " done" : "");
      } else {
        el.querySelector(".info-idx").textContent =
          pane.dir_name + "  " + (pane.image_index + 1) + "/" + state.total_images;
      }

      // Dim panes that have finished their chunk
      if (state.speed_mode && pane.chunk_done) {
        el.classList.add("chunk-done");
      } else {
        el.classList.remove("chunk-done");
      }
    });

    // Status bar
    const g = state.grid_rows + "x" + state.grid_cols;
    const fp = state.focused_pane + 1;
    const fpane = state.panes[state.focused_pane];
    const dirInfo = state.dir_count > 1 ? ("  |  Dir: " + fpane.dir_name) : "";
    const modeTag = state.speed_mode ? "SPEED" : "COMPARE";
    status.textContent =
      "[" + modeTag + "]  " +
      "Grid: " + g +
      "  |  Pane: " + fp +
      "  |  " + (fpane.image_index + 1) + "/" + state.total_images +
      dirInfo +
      "  |  " + fpane.image_name;

    preloadAdjacent();
  }

  // --- Keyboard handling ---
  document.addEventListener("keydown", function (e) {
    // Goto dialog mode
    if (gotoMode) {
      if (e.key === "Enter") {
        const val = parseInt(gotoInput.value, 10);
        if (!isNaN(val) && val >= 1) {
          send({ cmd: "goto", index: val - 1 });
        }
        gotoMode = false;
        gotoDialog.classList.add("hidden");
        gotoInput.value = "";
        e.preventDefault();
        return;
      }
      if (e.key === "Escape") {
        gotoMode = false;
        gotoDialog.classList.add("hidden");
        gotoInput.value = "";
        e.preventDefault();
        return;
      }
      return; // let input handle typing
    }

    const key = e.key;

    // Prevent browser defaults for our keys
    if (["Tab", "+", "=", "-", "ArrowLeft", "ArrowRight", "ArrowUp", "ArrowDown", "Enter", "[", "]"].includes(key)) {
      e.preventDefault();
    }

    switch (key) {
      case "l":
      case "d":
      case "ArrowRight":
        send({ cmd: "next" });
        break;
      case "h":
      case "a":
      case "ArrowLeft":
        send({ cmd: "prev" });
        break;
      case "j":
        send({ cmd: "jump", delta: 10 });
        break;
      case "k":
        send({ cmd: "jump", delta: -10 });
        break;
      case "J":
        send({ cmd: "jump", delta: 100 });
        break;
      case "K":
        send({ cmd: "jump", delta: -100 });
        break;
      case "]":
        send({ cmd: "next_dir" });
        break;
      case "[":
        send({ cmd: "prev_dir" });
        break;
      case "Enter":
        if (state && state.speed_mode) {
          if (e.shiftKey) {
            send({ cmd: "retreat_all" });
          } else {
            send({ cmd: "advance_all" });
          }
        } else {
          send({ cmd: "enter_speed" });
        }
        break;
      case "+":
      case "=":
        send({ cmd: "grid_up" });
        break;
      case "-":
      case "_":
        send({ cmd: "grid_down" });
        break;
      case "Tab":
        if (e.shiftKey) {
          send({ cmd: "focus_prev" });
        } else {
          send({ cmd: "focus_next" });
        }
        break;
      case "1": case "2": case "3": case "4":
      case "5": case "6": case "7": case "8": case "9":
        send({ cmd: "focus", pane: parseInt(key, 10) - 1 });
        break;
      case "g":
        gotoMode = true;
        gotoDialog.classList.remove("hidden");
        gotoInput.focus();
        e.preventDefault();
        break;
      case "f":
        if (!document.fullscreenElement) {
          document.documentElement.requestFullscreen();
        } else {
          document.exitFullscreen();
        }
        break;
      case "?":
        helpEl.classList.toggle("hidden");
        break;
      case "q":
        send({ cmd: "quit" });
        break;
      case "Escape":
        if (!helpEl.classList.contains("hidden")) {
          helpEl.classList.add("hidden");
        } else if (state && state.speed_mode) {
          send({ cmd: "exit_speed" });
        }
        break;
    }
  });

  // Click on pane to focus
  // (handled in render via event listeners)

  connect();
})();
