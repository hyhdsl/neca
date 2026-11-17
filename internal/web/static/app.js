(function () {
  "use strict";
  var key = "neca-theme";
  var root = document.documentElement;
  function applyTheme(value) {
    if (value === "light" || value === "dark") {
      root.setAttribute("data-theme", value);
    } else {
      root.setAttribute("data-theme", "auto");
    }
  }
  function saveTheme(value) {
    try { localStorage.setItem(key, value); } catch (e) {}
  }
  function readTheme() {
    try { return localStorage.getItem(key) || "system"; } catch (e) { return "system"; }
  }
  var saved = readTheme();
  applyTheme(saved);
  var select = document.getElementById("theme-select");
  if (select) {
    select.value = saved === "light" || saved === "dark" ? saved : "system";
    select.addEventListener("change", function () {
      applyTheme(select.value);
      saveTheme(select.value);
    });
  }
  var autoSubmit = document.querySelectorAll("[data-auto-submit]");
  for (var i = 0; i < autoSubmit.length; i++) {
    autoSubmit[i].addEventListener("change", function () {
      if (this.form) { this.form.submit(); }
    });
  }
})();
