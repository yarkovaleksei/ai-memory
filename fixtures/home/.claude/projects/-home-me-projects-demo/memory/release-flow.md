---
name: release-flow
description: Релиз выпускается тегом vX.Y.Z из main
metadata:
  type: project
  node_type: memory
---

Релиз — аннотированный тег `vX.Y.Z` на `main`, changelog собирается из коммитов. Ветку для стабилизации называют release.

**Why:** так договорились в команде 2026-01-02.

**How to apply:** перед тегом обнови CHANGELOG.md. Связано с [[prefer-yarn]].
