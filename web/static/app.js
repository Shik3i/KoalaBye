document.documentElement.classList.add("js");

(function() {
    var theme = localStorage.getItem("theme");
    if (theme === "light" || theme === "dark") {
        document.documentElement.setAttribute("data-theme", theme);
    }

    window.addEventListener("DOMContentLoaded", function() {
        var toggles = document.querySelectorAll("[data-theme-selector]");
        toggles.forEach(function(switcher) {
            if (theme === "light" || theme === "dark") {
                switcher.value = theme;
            } else {
                switcher.value = "system";
            }

            switcher.addEventListener("change", function() {
                var val = switcher.value;
                if (val === "light" || val === "dark") {
                    localStorage.setItem("theme", val);
                    document.documentElement.setAttribute("data-theme", val);
                } else {
                    localStorage.removeItem("theme");
                    document.documentElement.removeAttribute("data-theme");
                }
                
                toggles.forEach(function(other) {
                    if (other !== switcher) other.value = val;
                });
            });
        });

        document.querySelectorAll("[data-copy-target]").forEach(function(button) {
            button.addEventListener("click", function() {
                var target = document.getElementById(button.getAttribute("data-copy-target"));
                if (!target || !navigator.clipboard) return;
                navigator.clipboard.writeText(target.textContent).then(function() {
                    var original = button.textContent;
                    button.textContent = button.getAttribute("data-copy-label") || original;
                    window.setTimeout(function() { button.textContent = original; }, 1600);
                });
            });
        });
    });
})();
