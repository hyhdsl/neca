(function () {
  "use strict";
  var key = "neca-theme";
  var root = document.documentElement;
  try {
    var saved = localStorage.getItem(key);
    if (saved === "light" || saved === "dark") {
      root.setAttribute("data-theme", saved);
    }
  } catch (e) {}
  var button = document.getElementById("theme-toggle");
  if (button) {
    button.addEventListener("click", function () {
      var current = root.getAttribute("data-theme");
      if (current !== "light" && current !== "dark") {
        current = window.matchMedia("(prefers-color-scheme: dark)").matches ? "dark" : "light";
      }
      var next = current === "dark" ? "light" : "dark";
      root.setAttribute("data-theme", next);
      try { localStorage.setItem(key, next); } catch (e) {}
      button.setAttribute("aria-label", next === "dark" ? "切换到浅色主题" : "切换到深色主题");
    });
  }
})();
