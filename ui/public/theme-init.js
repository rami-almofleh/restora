// Runs before the first paint so the page never flashes the wrong theme or direction.
// Keep the storage keys in sync with src/theme/theme.ts and src/i18n/index.ts.
(function () {
  var root = document.documentElement;
  try {
    var theme = localStorage.getItem("restora.theme") || "system";
    var dark = theme === "dark" || (theme === "system" && window.matchMedia("(prefers-color-scheme: dark)").matches);
    root.setAttribute("data-theme", dark ? "dark" : "light");

    var lang = localStorage.getItem("restora.language");
    if (lang === "de" || lang === "en" || lang === "ar") {
      root.setAttribute("lang", lang);
      root.setAttribute("dir", lang === "ar" ? "rtl" : "ltr");
    }
  } catch (e) {
    root.setAttribute("data-theme", "light");
  }
})();
