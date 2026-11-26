// 本文件实现 Web 面板的浏览器端主题与交互。
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
  var form = document.getElementById("high-risk-form");
  if (form) {
    var dialog = document.getElementById("high-risk-dialog");
    var save = document.getElementById("high-risk-save");
    var readonly = form.querySelector('[name="readOnly"]');
    var readonlyWarning = document.getElementById("readonly-warning");
    var dirty = false;
    function markDirty() { dirty = true; }
    function updateReadonlyWarning() {
      if (readonly && readonlyWarning) { readonlyWarning.hidden = readonly.value !== "true"; }
    }
    if (readonly) { readonly.addEventListener("change", updateReadonlyWarning); updateReadonlyWarning(); }
    form.addEventListener("input", markDirty);
    form.addEventListener("change", markDirty);
    form.addEventListener("submit", function () { dirty = false; });
    if (save && dialog) {
      save.addEventListener("click", function () {
        if (typeof dialog.showModal === "function") { dialog.showModal(); }
        else { dialog.setAttribute("open", ""); }
      });
      var closers = dialog.querySelectorAll("[data-dialog-close]");
      for (var j = 0; j < closers.length; j++) {
        closers[j].addEventListener("click", function () {
          if (typeof dialog.close === "function") { dialog.close(); }
          else { dialog.removeAttribute("open"); }
        });
      }
    }
    window.addEventListener("beforeunload", function (event) {
      if (!dirty) { return; }
      event.preventDefault();
      event.returnValue = form.getAttribute("data-unsaved-message") || "";
      return event.returnValue;
    });
  }
})();
