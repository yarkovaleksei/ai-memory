#!/bin/sh

# Профиль AppArmor (только в deb, ADR-028): загрузить сразу, чтобы окно
# запускалось из меню без перезагрузки. Где AppArmor нет или он выключен,
# ничего не делаем.
if [ -f /etc/apparmor.d/ai-memory ] && command -v apparmor_parser >/dev/null 2>&1 &&
  [ "$(cat /sys/module/apparmor/parameters/enabled 2>/dev/null)" = "Y" ]; then
  apparmor_parser -r -T -W /etc/apparmor.d/ai-memory ||
    echo "Warning: AppArmor profile for ai-memory was not loaded." >&2
fi

# Update desktop database for .desktop file changes
# This makes the application appear in application menus and registers its capabilities.
if command -v update-desktop-database >/dev/null 2>&1; then
  echo "Updating desktop database..."
  update-desktop-database -q /usr/share/applications
else
  echo "Warning: update-desktop-database command not found. Desktop file may not be immediately recognized." >&2
fi

# Update MIME database for custom URL schemes (x-scheme-handler)
# This ensures the system knows how to handle your custom protocols.
if command -v update-mime-database >/dev/null 2>&1; then
  echo "Updating MIME database..."
  update-mime-database -n /usr/share/mime
else
  echo "Warning: update-mime-database command not found. Custom URL schemes may not be immediately recognized." >&2
fi

exit 0
