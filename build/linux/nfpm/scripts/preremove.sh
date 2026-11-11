#!/bin/sh

# Выгрузить профиль AppArmor при удалении пакета (ADR-028). При обновлении
# ($1 = upgrade у deb) профиль остаётся: новая версия загрузит свой.
if [ "$1" = "remove" ] && [ -f /etc/apparmor.d/ai-memory ] && command -v apparmor_parser >/dev/null 2>&1 &&
  [ "$(cat /sys/module/apparmor/parameters/enabled 2>/dev/null)" = "Y" ]; then
  apparmor_parser -R /etc/apparmor.d/ai-memory 2>/dev/null || true
fi

exit 0
