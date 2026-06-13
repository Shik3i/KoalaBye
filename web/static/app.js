document.documentElement.classList.add("js");

(function() {
    var theme = localStorage.getItem("theme");
    if (theme === "light" || theme === "dark") {
        document.documentElement.setAttribute("data-theme", theme);
    }

    window.addEventListener("DOMContentLoaded", function() {
        var toggles = document.querySelectorAll(".theme-toggle");
        toggles.forEach(function(btn) {
            btn.addEventListener("click", function() {
                var current = document.documentElement.getAttribute("data-theme");
                var isDark = current === "dark";
                if (!current) {
                    isDark = window.matchMedia && window.matchMedia('(prefers-color-scheme: dark)').matches;
                }
                var val = isDark ? "light" : "dark";
                localStorage.setItem("theme", val);
                document.documentElement.setAttribute("data-theme", val);
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

        document.querySelectorAll(".language-switcher select").forEach(function(el) {
            el.addEventListener("change", function() {
                el.form.submit();
            });
        });

        document.querySelectorAll("input[name='name']").forEach(function(nameInput) {
            var form = nameInput.closest("form");
            if (!form) return;
            var slugInput = form.querySelector("input[name='slug']");
            if (!slugInput) return;

            var autoGenerate = slugInput.value === "";

            nameInput.addEventListener("input", function() {
                if (autoGenerate) {
                    var val = nameInput.value.toLowerCase().replace(/[^a-z0-9]+/g, "-").replace(/^-|-$/g, "");
                    slugInput.value = val;
                }
            });

            slugInput.addEventListener("input", function() {
                autoGenerate = false;
                var start = this.selectionStart;
                var end = this.selectionEnd;
                this.value = this.value.toLowerCase();
                this.setSelectionRange(start, end);
            });
        });
    });
})();
