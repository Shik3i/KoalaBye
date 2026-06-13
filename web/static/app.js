document.documentElement.classList.add("js");

(function() {
    var theme = localStorage.getItem("theme");
    if (theme === "light" || theme === "dark") {
        document.documentElement.setAttribute("data-theme", theme);
    }

    window.addEventListener("DOMContentLoaded", function() {
        var toggles = document.querySelectorAll(".theme-toggle");
        toggles.forEach(function(btn) {
            var updateThemeButton = function() {
                var current = document.documentElement.getAttribute("data-theme");
                var dark = current === "dark" || (!current && window.matchMedia && window.matchMedia("(prefers-color-scheme: dark)").matches);
                btn.setAttribute("aria-pressed", dark ? "true" : "false");
            };
            btn.addEventListener("click", function() {
                var current = document.documentElement.getAttribute("data-theme");
                var isDark = current === "dark";
                if (!current) {
                    isDark = window.matchMedia && window.matchMedia('(prefers-color-scheme: dark)').matches;
                }
                var val = isDark ? "light" : "dark";
                localStorage.setItem("theme", val);
                document.documentElement.setAttribute("data-theme", val);
                updateThemeButton();
            });
            updateThemeButton();
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

        document.querySelectorAll("[data-confirm]").forEach(function(form) {
            form.addEventListener("submit", function(event) {
                if (!window.confirm(form.getAttribute("data-confirm"))) {
                    event.preventDefault();
                }
            });
        });

        document.querySelectorAll("[data-toast]").forEach(function(toast) {
            var close = toast.querySelector("[data-toast-close]");
            if (close) {
                close.addEventListener("click", function() {
                    toast.remove();
                });
            }
            window.setTimeout(function() {
                if (toast.isConnected) toast.remove();
            }, 6000);
        });

        document.querySelectorAll("[data-preset-preview]").forEach(function(preview) {
            var form = preview.closest("form");
            if (!form) return;
            var presetSelect = form.querySelector("[data-preset-select]");
            var presetRadios = form.querySelectorAll("[data-preset-radio]");
            var languageSelect = form.querySelector("select[name='public_language_default']");
            var updatePreview = function() {
                var preset = presetSelect ? presetSelect.value : "";
                presetRadios.forEach(function(radio) {
                    if (radio.checked) preset = radio.value;
                });
                var visibleLanguage = preview.querySelector("[data-preset-language]:not([hidden])");
                var language = languageSelect ? languageSelect.value : (visibleLanguage ? visibleLanguage.getAttribute("data-preset-language") : "");
                if (!language) language = "en";
                preview.querySelectorAll("[data-preset-panel]").forEach(function(panel) {
                    panel.hidden = panel.getAttribute("data-preset-panel") !== preset ||
                        panel.getAttribute("data-preset-language") !== language;
                });
            };
            if (presetSelect) presetSelect.addEventListener("change", updatePreview);
            presetRadios.forEach(function(radio) {
                radio.addEventListener("change", updatePreview);
            });
            if (languageSelect) languageSelect.addEventListener("change", updatePreview);
            updatePreview();
        });

        var fieldList = document.querySelector("[data-field-list]");
        var reorderForm = document.querySelector("[data-reorder-form]");
        if (fieldList && reorderForm) {
            var dragged = null;
            var saveButton = reorderForm.querySelector("[data-reorder-save]");
            var inputs = reorderForm.querySelector("[data-order-inputs]");
            var syncOrder = function() {
                inputs.textContent = "";
                fieldList.querySelectorAll("[data-field-id]").forEach(function(card) {
                    var input = document.createElement("input");
                    input.type = "hidden";
                    input.name = "field_order";
                    input.value = card.getAttribute("data-field-id");
                    inputs.appendChild(input);
                });
                saveButton.hidden = false;
            };
            fieldList.querySelectorAll("[data-field-id]").forEach(function(card) {
                card.addEventListener("dragstart", function(event) {
                    dragged = card;
                    card.classList.add("dragging");
                    event.dataTransfer.effectAllowed = "move";
                });
                card.addEventListener("dragend", function() {
                    card.classList.remove("dragging");
                    dragged = null;
                });
            });
            fieldList.addEventListener("dragover", function(event) {
                if (!dragged) return;
                event.preventDefault();
                var target = event.target.closest("[data-field-id]");
                if (!target || target === dragged) return;
                var bounds = target.getBoundingClientRect();
                fieldList.insertBefore(dragged, event.clientY < bounds.top + bounds.height / 2 ? target : target.nextSibling);
                syncOrder();
            });
        }
    });
})();
