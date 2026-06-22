document.documentElement.classList.add("js");

(function() {
    var theme = localStorage.getItem("theme");
    if (theme === "light" || theme === "dark") {
        document.documentElement.setAttribute("data-theme", theme);
    }

    window.addEventListener("DOMContentLoaded", function() {
        var commandDialog = document.querySelector("[data-command-dialog]");
        var commandOpen = document.querySelector("[data-command-open]");
        var commandClose = document.querySelector("[data-command-close]");
        var commandInput = document.querySelector("[data-command-input]");
        var commandReturnFocus = null;
        var openCommand = function() {
            if (!commandDialog || !commandDialog.showModal) return;
            commandReturnFocus = document.activeElement;
            commandDialog.showModal();
            window.setTimeout(function() { if (commandInput) commandInput.focus(); }, 0);
        };
        var closeCommand = function() {
            if (!commandDialog || !commandDialog.open) return;
            commandDialog.close();
            if (commandReturnFocus && commandReturnFocus.focus) commandReturnFocus.focus();
        };
        if (commandOpen && commandDialog) {
            commandOpen.addEventListener("click", function(event) {
                event.preventDefault();
                openCommand();
            });
        }
        if (commandClose) commandClose.addEventListener("click", closeCommand);
        if (commandDialog) {
            commandDialog.addEventListener("click", function(event) {
                if (event.target === commandDialog) closeCommand();
            });
        }
        document.addEventListener("keydown", function(event) {
            if ((event.metaKey || event.ctrlKey) && event.key.toLowerCase() === "k") {
                event.preventDefault();
                openCommand();
            } else if (event.key === "Escape" && commandDialog && commandDialog.open) {
                event.preventDefault();
                closeCommand();
            }
        });

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
        var publicForm = document.querySelector("[data-public-feedback-form]");
        if (publicForm) {
            var startRecorded = false;
            var recordStart = function() {
                if (startRecorded) return;
                startRecorded = true;
                var visit = publicForm.querySelector("input[name='visit_public_id']");
                var startURL = publicForm.getAttribute("data-start-url");
                if (!visit || !visit.value || !startURL) return;
                var body = new URLSearchParams();
                body.set("visit_public_id", visit.value);
                fetch(startURL, {
                    method: "POST",
                    headers: {"Content-Type": "application/x-www-form-urlencoded;charset=UTF-8"},
                    body: body.toString(),
                    credentials: "same-origin",
                    keepalive: true
                }).catch(function() {});
            };
            var inputs = publicForm.querySelectorAll("input[name^='field_'], textarea[name^='field_']");
            var fieldNames = new Set();
            inputs.forEach(function(el) {
                fieldNames.add(el.name);
                el.addEventListener("input", recordStart, {once: true});
                el.addEventListener("change", recordStart, {once: true});
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

        document.querySelectorAll(".page form[method='post'].form-stack").forEach(function(form) {
            if (form.hasAttribute("data-no-dirty-guard")) return;
            var initial = new FormData(form);
            var snapshot = function() {
                return Array.from(new FormData(form).entries()).map(function(entry) {
                    return entry[0] + "=" + entry[1];
                }).join("&");
            };
            var initialValue = Array.from(initial.entries()).map(function(entry) {
                return entry[0] + "=" + entry[1];
            }).join("&");
            var submitted = false;
            form.addEventListener("submit", function() {
                submitted = true;
                form.setAttribute("aria-busy", "true");
                form.querySelectorAll("button[type='submit']").forEach(function(button) {
                    button.disabled = true;
                });
            });
            window.addEventListener("beforeunload", function(event) {
                if (!submitted && snapshot() !== initialValue) {
                    event.preventDefault();
                    event.returnValue = "";
                }
            });
        });

        document.body.addEventListener("htmx:beforeRequest", function(event) {
            var target = event.target;
            target.setAttribute("aria-busy", "true");
            target.querySelectorAll("button, input, select, textarea").forEach(function(control) {
                control.dataset.htmxWasDisabled = control.disabled ? "true" : "false";
                control.disabled = true;
            });
        });
        document.body.addEventListener("htmx:afterRequest", function(event) {
            var target = event.target;
            target.removeAttribute("aria-busy");
            target.querySelectorAll("[data-htmx-was-disabled]").forEach(function(control) {
                control.disabled = control.dataset.htmxWasDisabled === "true";
                delete control.dataset.htmxWasDisabled;
            });
        });

        document.querySelectorAll("[data-toast]").forEach(function(toast) {
            var close = toast.querySelector("[data-toast-close]");
            var timer = null;
            var dismiss = function() {
                if (toast.isConnected) toast.remove();
            };
            var startTimer = function() {
                if (timer) clearTimeout(timer);
                timer = window.setTimeout(dismiss, 6000);
            };
            var stopTimer = function() {
                if (timer) { clearTimeout(timer); timer = null; }
            };
            if (close) {
                close.addEventListener("click", function() {
                    stopTimer();
                    dismiss();
                });
            }
            toast.addEventListener("mouseenter", stopTimer);
            toast.addEventListener("mouseleave", startTimer);
            startTimer();
        });

        document.querySelectorAll("[data-password-toggle]").forEach(function(wrapper) {
            var input = wrapper.querySelector("input[type='password'], input[type='text']");
            var toggle = wrapper.querySelector("[data-password-btn]");
            if (!input || !toggle) return;
            toggle.addEventListener("click", function() {
                var isPassword = input.type === "password";
                input.type = isPassword ? "text" : "password";
                toggle.setAttribute("aria-label", isPassword ? toggle.getAttribute("data-label-hide") : toggle.getAttribute("data-label-show"));
                var open = toggle.querySelector(".eye-open");
                var closed = toggle.querySelector(".eye-closed");
                if (open) open.style.display = isPassword ? "none" : "";
                if (closed) closed.style.display = isPassword ? "" : "none";
            });
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

        // Local timezone formatting
        var formatLocalTimes = function(root) {
            (root || document).querySelectorAll("time[data-local-time]").forEach(function(el) {
                var dtStr = el.getAttribute("datetime");
                if (!dtStr) return;
                var date = new Date(dtStr);
                if (isNaN(date.getTime())) return;

                var lang = document.documentElement.lang || "en";
                var pad = function(n) { return String(n).padStart(2, '0'); };
                var day = pad(date.getDate());
                var month = pad(date.getMonth() + 1);
                var year = String(date.getFullYear()).slice(-2);
                var hours = pad(date.getHours());
                var minutes = pad(date.getMinutes());

                if (lang.indexOf("de") === 0) {
                    el.textContent = day + "." + month + "." + year + " " + hours + ":" + minutes + " Uhr";
                } else {
                    el.textContent = day + "/" + month + "/" + year + ", " + hours + ":" + minutes;
                }
            });
        };
        formatLocalTimes();
        document.body.addEventListener("htmx:afterSettle", function(event) {
            formatLocalTimes(event.target);
        });

        // General double-submit protection: disable submit buttons on submit
        document.querySelectorAll("form[method='post'].form-stack").forEach(function(form) {
            form.addEventListener("submit", function() {
                form.setAttribute("aria-busy", "true");
                form.querySelectorAll("button[type='submit']").forEach(function(button) {
                    button.disabled = true;
                });
            });
        });

        // Handle localStorage submission tracking
        var successIndicator = document.querySelector("[data-submission-success]");
        if (successIndicator) {
            var campaignId = successIndicator.getAttribute("data-submission-success");
            if (campaignId) {
                localStorage.setItem("koalabye_submitted_" + campaignId, "true");
            }
        }

        var publicForm = document.querySelector("[data-public-feedback-form]");
        if (publicForm) {
            var campaignId = publicForm.getAttribute("data-campaign-id");
            if (campaignId && localStorage.getItem("koalabye_submitted_" + campaignId) === "true") {
                var formPanel = document.getElementById("feedback-form-panel");
                var thanksPanel = document.getElementById("feedback-already-submitted-panel");
                if (formPanel && thanksPanel) {
                    formPanel.style.display = "none";
                    thanksPanel.style.display = "block";
                }
            }
        }

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

