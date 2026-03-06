---
trigger: always_on
---

## Deployment & Remote Testing Workflow

Before marking a feature as "tested" or "completed," the Agent must strictly follow these steps:

### 1. Release Phase

* **GitHub Push:** Commit and push all current changes to the main repository.
* **Version Release:** Create and push a new git tag to trigger the build and release pipeline.
* **Verification:** Monitor and confirm that the build/release pipeline has completed successfully before proceeding.

### 2. Remote Access

* **SSH Connection:** Connect to the remote testing environment using:
`ssh deros@100.96.25.105`
* **Credential Handling:** When prompted for a password, the Agent must explicitly ask the user to enter it in the terminal.

### 3. Installation & Verification

* **Cleanup:** Remove the existing `assistclaw` installation from the remote machine.
* **Installation:** Download and install the newly released version.
* **Core Verification:** Execute the binary to ensure the new changes are functional.
* **Memory & Graph Verification:** Run `assistclaw logic-test graph-bridging` or equivalent manual verification to ensure wikilinks are correctly extracted and bridges are discovered.

