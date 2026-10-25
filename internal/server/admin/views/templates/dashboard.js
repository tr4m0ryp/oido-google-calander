// Copy-to-clipboard buttons and remove confirmation. Reads text from the
// DOM only; no server data is interpolated into this script.
(function () {
  document.querySelectorAll("[data-copy]").forEach(function (btn) {
    var label = btn.querySelector("[data-copy-label]");
    var original = label ? label.textContent : "";
    btn.addEventListener("click", function () {
      var el = document.getElementById(btn.getAttribute("data-copy"));
      if (!el) return;
      var text = "value" in el ? el.value : el.textContent;
      var done = function () {
        if (!label) return;
        label.textContent = "Copied";
        setTimeout(function () { label.textContent = original; }, 1600);
      };
      if (navigator.clipboard && window.isSecureContext) {
        navigator.clipboard.writeText(text).then(done, function () {});
        return;
      }
      var range = document.createRange();
      range.selectNodeContents(el);
      var sel = window.getSelection();
      sel.removeAllRanges();
      sel.addRange(range);
      try { document.execCommand("copy"); done(); } catch (e) {}
    });
  });
  document.querySelectorAll("form[data-confirm]").forEach(function (form) {
    form.addEventListener("submit", function (event) {
      if (!confirm(form.getAttribute("data-confirm"))) event.preventDefault();
    });
  });
})();
