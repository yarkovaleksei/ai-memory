---
name: prefer-yarn
description: Зависимости фронтенда ставятся только через yarn 4
metadata:
  type: feedback
  originSessionId: 00000000-0000-0000-0000-000000000001
  modified: 2026-01-02T10:00:00.000Z
---

Ставь и запускай фронтенд только через yarn, не через npm.

**Why:** в проекте yarn.lock, npm создаёт второй lock-файл.

**How to apply:** `yarn add`, `yarn test`; `npm` и `npx` не вызывать.
