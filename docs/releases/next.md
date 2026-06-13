# Next Experimental Release

This document tracks privacy and diagnostics improvements planned for the next experimental release after v0.1.2. It is not a release announcement and does not assign or create a tag.

## Implemented

- public campaign pages always disclose enabled collection
- opt-in allowlisted URL context for app, extension, platform, source, channel, and UTM values
- raw query strings and unknown context keys are discarded
- authorized CSV and JSON exports can include sanitized linked context
- structured privacy-policy placeholders for operators
- expanded privacy, security, deployment, testing, and release documentation

## Deliberately Deferred

- partial-response endpoint and abandoned-form events
- text draft autosave
- browser major-version storage
- device-class storage
- coarse UTC-offset collection
- custom URL context keys

These items remain deferred until their public disclosure, data model, validation, retention, and abuse controls can be implemented without weakening KoalaBye's privacy posture.
