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
                
                if (bodyLabel) bodyLabel.classList.toggle("hidden", val !== "text_block");
                if (maxLengthLabel) maxLengthLabel.classList.toggle("hidden", val !== "textarea");
                if (requiredLabel) requiredLabel.classList.toggle("hidden", val === "text_block");
            };
            
            fieldTypeSelect.addEventListener("change", updateFieldVisibility);
            updateFieldVisibility();
        }

        // Branding form live preview
        var brandingForm = document.getElementById("branding-form");
        if (brandingForm) {
            var previewBody = document.getElementById("preview-body");
            var previewBrandName = document.getElementById("preview-brand-name");
            var previewHeading = document.getElementById("preview-heading");
            var previewIntro = document.getElementById("preview-intro");
            var brandNameInput = brandingForm.querySelector('[name="brand_name"]');
            var headingInput = brandingForm.querySelector('[name="public_heading"]');
            var introInput = brandingForm.querySelector('[name="public_intro"]');

            function updateBrandingPreview() {
                var accent = brandingForm.querySelector('[name="accent_preset"]').value || "default";
                var theme = brandingForm.querySelector('[name="background_style"]').value || "theme-default";
                if (previewBody) previewBody.className = "public-body accent-" + accent + " " + theme;
                if (previewBrandName) previewBrandName.textContent = brandNameInput ? (brandNameInput.value || "KoalaBye") : "KoalaBye";
                if (previewHeading) previewHeading.textContent = headingInput ? (headingInput.value || previewHeading.dataset.fallback) : previewHeading.dataset.fallback;
                if (previewIntro) previewIntro.textContent = introInput ? (introInput.value || previewIntro.dataset.fallback) : previewIntro.dataset.fallback;
            }

            brandingForm.addEventListener("input", updateBrandingPreview);
            brandingForm.addEventListener("change", updateBrandingPreview);
            updateBrandingPreview();
        }

        // Subnav aria-current detection
        document.querySelectorAll(".subnav a").forEach(function(link) {
            if (link.getAttribute("href") === window.location.pathname) {
                link.setAttribute("aria-current", "page");
            }
        });

        // Form submit loading state
        document.querySelectorAll("form").forEach(function(form) {
            form.addEventListener("submit", function() {
                var btn = form.querySelector('button[type="submit"]');
                if (btn) { btn.disabled = true; btn.setAttribute("aria-busy", "true"); }
            });
        });
    });
})();
