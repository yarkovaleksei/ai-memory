package updates

import "strings"

// Format — формат пакета, из которого установлена эта сборка. Известен из
// сборки, а не угадывается по окружению (ADR-031): в dmg и nsis он вшит
// через -X main.packageFormat, пакеты Linux ставят метку LinuxMarker — по
// одному бинарю не отличить deb от rpm.
type Format string

const (
	FormatNone Format = "" // сборка из исходников или wails3 task dev
	FormatDeb  Format = "deb"
	FormatRPM  Format = "rpm"
	FormatArch Format = "arch"
	FormatDMG  Format = "dmg"
	FormatNSIS Format = "nsis"
)

// ParseFormat принимает значение из сборки; неизвестное — FormatNone, то есть
// обновление вручную.
func ParseFormat(s string) Format {
	switch f := Format(s); f {
	case FormatDeb, FormatRPM, FormatArch, FormatDMG, FormatNSIS:
		return f
	}
	return FormatNone
}

// Install — как ставится скачанный пакет. Код уходит во фронтенд, текст
// подсказки собирается там по словарю.
type Install string

const (
	// InstallCommand — пакет ставит пользователь командой с правами root;
	// приложение открывает папку и показывает команду.
	InstallCommand Install = "command"
	// InstallOpen — пакет открывается системой (.dmg в Finder).
	InstallOpen Install = "open"
	// InstallRun — запускается установщик, окно закрывается.
	InstallRun Install = "run"
)

// Contract — имена вложений релиза и способ установки для формата. Имена —
// те же, под которыми пакеты выкладывает выпуск (Taskfile.yml release,
// .github/workflows/release.yml, scripts/release-verify.sh): это контракт
// между выпуском и приложением, меняются только вместе.
type Contract struct {
	Asset   string  // имя пакета во вложениях релиза
	Sums    string  // SHA256SUMS-<система>.txt, где лежит его сумма
	Install Install // как ставить
	Command string  // команда установки для InstallCommand, путь — в конце
}

// ContractFor — контракт формата для версии X.Y.Z. ok=false у FormatNone.
func ContractFor(f Format, v string) (Contract, bool) {
	switch f {
	case FormatDeb:
		return Contract{"ai-memory_" + v + "_amd64.deb", "SHA256SUMS-linux.txt", InstallCommand, "sudo apt install"}, true
	case FormatRPM:
		return Contract{"ai-memory-" + v + ".x86_64.rpm", "SHA256SUMS-linux.txt", InstallCommand, "sudo dnf install"}, true
	case FormatArch:
		return Contract{"ai-memory-" + v + "-x86_64.pkg.tar.zst", "SHA256SUMS-linux.txt", InstallCommand, "sudo pacman -U"}, true
	case FormatDMG:
		return Contract{"ai-memory-" + v + "-macos-universal.dmg", "SHA256SUMS-macos.txt", InstallOpen, ""}, true
	case FormatNSIS:
		return Contract{"ai-memory-" + v + "-windows-amd64-setup.exe", "SHA256SUMS-windows.txt", InstallRun, ""}, true
	}
	return Contract{}, false
}

// LinuxMarker — метка формата, которую ставит пакет Linux (build/linux/nfpm):
// deb, rpm и пакет Arch несут один и тот же бинарь.
const LinuxMarker = "/usr/share/ai-memory/package-format"

// linuxBinary — куда пакеты Linux ставят окно.
const linuxBinary = "/usr/bin/ai-memory"

// DetectFormat — формат этой сборки. built — вшитый при сборке
// (main.packageFormat: dmg, nsis). На Linux метку читает только бинарь из
// /usr/bin: собранный из исходников рядом с установленным пакетом не должен
// считать себя пакетом.
func DetectFormat(built, goos, exe string, read func(string) ([]byte, error)) Format {
	if f := ParseFormat(built); f != FormatNone {
		return f
	}
	if goos != "linux" || exe != linuxBinary {
		return FormatNone
	}
	data, err := read(LinuxMarker)
	if err != nil {
		return FormatNone
	}
	switch f := ParseFormat(strings.TrimSpace(string(data))); f {
	case FormatDeb, FormatRPM, FormatArch:
		return f
	}
	return FormatNone
}
