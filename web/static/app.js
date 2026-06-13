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

        // Auto-submit public forms on choice selection (radio/rating) if all fields are answered
        var publicForm = document.querySelector(".public-page form");
        if (publicForm) {
            var inputs = publicForm.querySelectorAll("input[name^='field_'], textarea[name^='field_']");
            var fieldNames = new Set();
            inputs.forEach(function(el) {
                fieldNames.add(el.name);
            });

            publicForm.querySelectorAll("input[type='radio']").forEach(function(radio) {
                radio.addEventListener("change", function() {
                    var allAnswered = true;
                    fieldNames.forEach(function(name) {
                        var fieldInputs = publicForm.querySelectorAll("[name='" + name + "']");
                        var isAnswered = false;
                        fieldInputs.forEach(function(fInput) {
                            if (fInput.type === "radio" || fInput.type === "checkbox") {
                                if (fInput.checked) isAnswered = true;
                            } else if (fInput.value.trim() !== "") {
                                isAnswered = true;
                            }
                        });
                        if (!isAnswered) {
                            allAnswered = false;
                        }
                    });
                    if (allAnswered) {
                        publicForm.submit();
                    }
                });
            });
        }

        // Dynamic visibility for conditional inputs in Form Builder "Add Field" form
        var fieldTypeSelect = document.querySelector("select[name='field_type']");
        if (fieldTypeSelect) {
            var form = fieldTypeSelect.closest("form");
            var updateFieldVisibility = function() {
                var val = fieldTypeSelect.value;
                
                var bodyLabel = form.querySelector(".conditional-body");
                var maxLengthLabel = form.querySelector(".conditional-max-length");
                var requiredLabel = form.querySelector(".conditional-required");
                
                if (bodyLabel) bodyLabel.style.display = (val === "text_block") ? "grid" : "none";
                if (maxLengthLabel) maxLengthLabel.style.display = (val === "textarea") ? "grid" : "none";
                if (requiredLabel) requiredLabel.style.display = (val === "text_block") ? "none" : "flex";
            };
            
            fieldTypeSelect.addEventListener("change", updateFieldVisibility);
            updateFieldVisibility();
        }
    });
})();
