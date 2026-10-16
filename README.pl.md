# Conflux

[English](README.md) · [Architektura](docs/ARCHITECTURE.md) · [Bezpieczeństwo](docs/SECURITY.md) · [Stan odbioru](docs/ACCEPTANCE.md)

Conflux to klient VPN IPv4 dla Windows 11 x64 i przekaźnik dla Ubuntu 24.04. Korzysta z własnych węzłów VLESS obsługujących UDP: wiele zwykłych połączeń QUIC przenosi jeden logiczny tunel do wspólnego przekaźnika. Serwisy internetowe widzą publiczny adres IP przekaźnika. Interfejs Wails ma język polski i angielski oraz motyw jasny, ciemny i systemowy; uprzywilejowana usługa zarządza siecią.

**To kod przed pierwszym wydaniem. Nie deklarujemy opublikowanych plików binarnych ani podpisanego instalatora. Pełny MVP nie przeszedł odbioru na rzeczywistych Windows/Ubuntu: instalacji, wycieków, awarii/restartu, agregacji przez dwa proxy i długich testów.** Używaj maszyny testowej z dostępem do konsoli. Przeczytaj [ograniczenia](docs/LIMITATIONS.md) i [macierz odbioru](docs/ACCEPTANCE.md).

```text
Aplikacje Windows → Wintun / usługa → osobny proces sing-box
                                      ├→ VLESS A / QUIC ─┐
                                      └→ VLESS B / QUIC ─┤
                                                        ↓
                                         wspólny przekaźnik Linux
                                           → TUN → nftables → Internet
```

Bond wymaga przynajmniej dwóch faktycznie przetestowanych ścieżek. Poprawny link lub działające proxy TCP nie wystarcza: muszą działać SOCKS UDP i uwierzytelnione QUIC do przekaźnika. Harmonogram rozdziela pakiety, a ograniczony bufor łagodzi zmianę kolejności. Awaria ścieżki powinna wyłączyć ją z harmonogramu; powrót następuje po ponownej ocenie stanu. Potwierdzenie działania wymaga testu na docelowym systemie.

Dwa proxy korzystające z jednego łącza ISP nie zwielokrotnią jego fizycznej przepustowości. Możliwe korzyści to obejście ograniczeń serwerów, mniej przeciążeń i odporność, jeżeli istnieje wolna przepustowość. MVP tuneluje IPv4; IPv6 nie jest tunelowane, a odwracalna ochrona przed wyciekiem wymaga testów Windows. Projekt nie zawiera analityki, reklam, wysyłania raportów awarii ani telemetrii. Diagnostyka pozostaje lokalna, dopóki użytkownik sam jej nie udostępni.

## Szybka instalacja

Potrzebujesz dwóch rzeczy: serwera relay (VPS z Ubuntu 24.04) i klienta na Windows 11. Każde to jedna komenda.

**1. Relay** — na świeżym VPS z Ubuntu 24.04 x86_64, z rootem, TUN i otwartym przychodzącym UDP 4433:

```sh
curl -fsSL https://raw.githubusercontent.com/filiprek/conflux-vpn/main/scripts/setup-relay.sh | sudo bash
```

Skrypt buduje relay ze zweryfikowanym sumą kontrolną Go, instaluje usługę systemd i **jednorazowo** wypisuje tajną linię `conflux://...`. Zapisz ją w bezpiecznym miejscu. Dopisz `-s -- --public-host relay.example.org`, aby użyć nazwy DNS zamiast automatycznie wykrytego publicznego IPv4. Otwórz przychodzący UDP 4433 w firewallu dostawcy chmury.

**2. Klient Windows** — w zwykłym (nie administratorskim) oknie PowerShell, jako użytkownik, który będzie korzystał z VPN:

```powershell
& ([scriptblock]::Create((irm https://raw.githubusercontent.com/filiprek/conflux-vpn/main/scripts/setup-windows.ps1)))
```

Skrypt w razie potrzeby instaluje Git, Go 1.27.2 i Node.js 24 dla Twojego konta, klonuje kod do `%USERPROFILE%\conflux-vpn`, buduje całość, pobiera sing-box 1.14.3 i Wintun 0.14.1 z oficjalnych stron (przypięte SHA-256, weryfikacja podpisu Wintun), instaluje usługę po jednym potwierdzeniu administratora i otwiera Conflux.

**3. Połączenie** — w Conflux: *Serwer relay* → wklej linię enrollment i potwierdź odcisk; *Ścieżki* → dodaj co najmniej dwa linki `vless://` i **Testuj** każdy; *Przegląd* → **Połącz**.

Ponowne uruchomienie komendy aktualizuje instalację. Parametr `-BuildOnly` w skrypcie Windows tylko buduje, bez instalacji.

## Ręczna kompilacja ze źródeł

Zainstaluj oficjalne Go **1.27.2** (https://go.dev/dl/), Node.js **24** z npm i Git. Po instalacji otwórz nowy terminal, aby `go` było w `PATH`; `go env GOVERSION` musi zwrócić `go1.27.2`.

Na Linuksie/macOS (bash i make):

```sh
git clone https://github.com/filiprek/conflux-vpn.git
cd conflux-vpn
go mod download
make test
make frontend
make build
```

Na Windows `make` ani bash nie są potrzebne. W PowerShell:

```powershell
git clone https://github.com/filiprek/conflux-vpn.git
cd conflux-vpn
./scripts/dev.ps1 test       # testy Go (moduł główny i GUI)
./scripts/dev.ps1 frontend   # npm ci, typecheck, lint, testy, build
./scripts/dev.ps1 build      # usługa, CLI, bench oraz GUI Conflux.exe
```

Jeżeli PowerShell blokuje uruchamianie skryptów, jednorazowo zezwól na lokalne skrypty dla swojego konta: `Set-ExecutionPolicy -Scope CurrentUser RemoteSigned`. `./scripts/dev.ps1 check` dodaje kontrolę gofmt/vet/składni PowerShell, a `./scripts/dev.ps1 security` uruchamia govulncheck i npm audit.

Wyniki: `dist/linux/conflux-relay`, `dist/linux/conflux-bench` i `dist/windows/{conflux-service,confluxctl,conflux-bench}.exe`. Kompilacja krzyżowa na Linuksie nie tworzy natywnego GUI Windows. `make check` uruchamia formatowanie kontrolne, testy, vet, race i kontrolę składni skryptów. `make security` uruchamia przypięty govulncheck i npm audit. [Instrukcja testów](docs/TESTING.md) opisuje osobny moduł GUI i testy administracyjne.

Budowanie GUI na Windows wymaga WebView2 (preinstalowany w Windows 11); `./scripts/dev.ps1 build` uruchamia `scripts/build-windows.ps1`, który sam pobiera przypiętą wersję Wails v2 CLI. Pakowanie:

```powershell
# Po osobnej instalacji NSIS 3.11:
makensis deploy/windows/conflux.nsi
```

GUI powstaje jako `dist/windows/Conflux.exe`, a niepodpisany instalator jako `dist/Conflux-unsigned-setup.exe`. [Pełna instalacja Windows](docs/WINDOWS_NETWORKING.md) opisuje weryfikację zależności i opcje. Samo zbudowanie kodu lub zapisanie konfiguracji CI nie potwierdza działania instalacji.

## Instalacja przekaźnika Ubuntu

Przenieś zbudowany plik Linuksa i cały katalog `deploy/relay` na VPS Ubuntu 24.04 x86_64 z root, TUN, publicznym IPv4 i dostępem UDP 4433. Na serwerze:

```sh
sudo bash deploy/relay/install-relay.sh \
  --binary /bezwzgledna/sciezka/conflux-relay \
  --public-host relay.example.org
```

Podstaw rzeczywistą nazwę DNS lub publiczny IPv4. Skrypt generuje certyfikat i losowy token, ustawia pliki sekretów na 0600, tworzy wyłącznie własny stan sieci oraz ograniczone usługi systemd. Po lokalnym uwierzytelnionym sprawdzeniu wypisuje sekretny link `conflux://...` **tylko raz**. Zapisz go prywatnie i porównaj odcisk certyfikatu niezależnym kanałem. Zezwól na UDP 4433 w zaporze VPS/hosta; istniejąca polityka przekazywania może wymagać jawnego wyjątku. Nie usuwaj obcych reguł. [Wdrożenie, rotacja tokenu i odzyskiwanie](docs/RELAY_DEPLOYMENT.md).

## Instalacja i użycie Windows

Pobierz osobny oficjalny sing-box **1.14.3**, niezależnie uwierzytelnij sumę kontrolną archiwum i zachowaj licencję GPL oraz informacje o odpowiadającym kodzie źródłowym. Pobierz oficjalny Wintun amd64 z https://www.wintun.net/. Sprawdź warunki dystrybucji, wiarygodną sumę i prawidłowy podpis Authenticode WireGuard. Projekt nie dołącza ani nie pobiera automatycznie tych plików. W tym środowisku nie zweryfikowano licencji/hashu gotowego Wintun; to przeszkoda do wydania. [Licencje zależności](THIRD_PARTY_NOTICES.md).

Przed podniesieniem uprawnień odczytaj SID docelowego użytkownika pulpitu: `[Security.Principal.WindowsIdentity]::GetCurrent().User.Value`. Następnie w PowerShell administratora użyj tego SID, rzeczywistych ścieżek i oczekiwanych sum SHA-256:

```powershell
./deploy/windows/install.ps1 -OwnerSID 'S-1-5-21-...' `
  -SourceDirectory "$PWD/dist/windows" `
  -SingBox 'C:/verified/sing-box.exe' -SingBoxSHA256 '<64 znaki szesnastkowe>' `
  -Wintun 'C:/verified/wintun.dll' -WintunSHA256 '<64 znaki szesnastkowe>'
```

Usługa działa uprzywilejowanie; GUI uruchamiaj jako wskazany zwykły użytkownik. Instalator zabezpiecza programy i dane ACL administratorów/SYSTEM. Wariant NSIS przyjmuje `/OWNER_SID=`, `/SINGBOX=`, `/SINGBOX_SHA256=`, `/WINTUN=`, `/WINTUN_SHA256=`. Jest niepodpisany; projekt nie deklaruje zaufanej tożsamości wydawcy.

1. Uruchom `%ProgramFiles%/Conflux/Conflux.exe` jako właściciel instalacji.
2. Prywatnie zaimportuj link przekaźnika i sprawdź odcisk przez zaufany kanał.
3. Dodaj własne linki `vless://`. Sprawdź nazwę, serwer, transport i błędy nieobsługiwanych pól. Nie wysyłaj linków w zgłoszeniach.
4. Przetestuj co najmniej dwa węzły pod kątem rzeczywistego UDP/QUIC i wybierz kwalifikujące się ścieżki Bond.
5. Połącz i poczekaj na stan potwierdzony przez usługę. Potwierdź liczniki obu ścieżek, adres wyjściowy przekaźnika, DNS w tunelu i brak wycieku IPv6.
6. Podczas kontrolowanego transferu wyłącz jedną ścieżkę, sprawdź kontynuację przez drugą i powrót pierwszej. Rozłącz i potwierdź odtworzenie sieci.

To kroki odbioru do wykonania, nie deklaracja uzyskanych wyników. [Wymagania laboratorium](docs/LAB_ACCEPTANCE.md) obejmują limity przepustowości, różne RTT, awarie i długie testy. Nie podajemy zmyślonych pomiarów.

## Odzyskiwanie i odinstalowanie

Jako właściciel/administrator użyj `confluxctl --pipe Conflux-<SID właściciela> status`, `disconnect`, `recover` i `diagnostics`. Import sekretów odbywa się przez stdin; GUI pozwala uniknąć historii powłoki. Przy błędzie zachowaj dziennik odtwarzania. [Rozwiązywanie problemów](docs/TROUBLESHOOTING.md).

Zamknij GUI, a następnie w podniesionym PowerShell:

```powershell
& "$env:ProgramFiles/Conflux/uninstall.ps1"
# Opcja -PurgeData świadomie usuwa chronione dane lokalne.
```

Odinstalowanie wymaga udanego rozłączenia i odzyskania sieci przed usunięciem usługi. Domyślnie dane pozostają. Jeśli użyto NSIS, uruchom zarejestrowany deinstalator, aby usunąć również wpisy instalatora. Nie wykonuje się globalnego resetu sieci ani usuwania obcych tras/sterowników.

Na przekaźniku:

```sh
sudo bash deploy/relay/uninstall-relay.sh
```

Usuwany jest wyłącznie własny stan usług/sieci. Przekazywanie IPv4 jest przywracane tylko wtedy, gdy bieżąca wartość nadal odpowiada zapisanej zmianie Conflux. Sekrety i ewidencja instalacji pozostają do jawnego usunięcia zgodnie z [polityką odinstalowania](docs/RELAY_DEPLOYMENT.md).

Kod projektu: Apache-2.0. Osobne programy zachowują własne licencje. [Współtworzenie](CONTRIBUTING.md) · [Zgłoszenia bezpieczeństwa](SECURITY.md) · [Historia zmian](CHANGELOG.md).
