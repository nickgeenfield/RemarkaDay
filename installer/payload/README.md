# Installer payload

The Windows installer embeds this directory (`//go:embed payload` in `main.go`). Two files are already here:

| File | Role |
| --- | --- |
| `manifest.json` | AppLoad manifest. The id stays `org.remarkablecalendar.move`. The name shown in AppLoad is reCalendar. |
| `icon.png` | 64×64 RGB icon, flattened onto white. AppLoad shows it next to the app name. The 256×256 original is `assets/icon.png`. |

Two further files are required before an install can succeed. They are listed in `.gitignore` because they are built from [nguyer/remarkable-calendar](https://github.com/nguyer/remarkable-calendar). That project has no licence file, so this repository does not store them.

| File | Ignored path |
| --- | --- |
| `resources.rcc` | `installer/payload/resources.rcc` |
| `backend/entry` | `installer/payload/backend/` |

Do not put secrets in this directory. The installer compiles everything here into the executable.

The patch and the commands below are for upstream commit `5ee822fffa0d2b7fe462cb73281fa29df3aaaeae`.

## 1. Check out upstream and apply the reCalendar patch

```sh
git clone https://github.com/nguyer/remarkable-calendar
cd remarkable-calendar
git checkout 5ee822fffa0d2b7fe462cb73281fa29df3aaaeae
git apply --exclude=prebuilt-ui/icon.png /path/to/reCalendar/patches/recalendar-changes.patch
```

`patches/recalendar-changes.patch` updates the QML (close button in the week toolbar, long-press to delete in the week and day views) and sets the AppLoad display name to reCalendar. The icon hunk only says that the binary differs; it does not contain the new pixels, so it is excluded. Use the icon already in this directory.

Apply the patch before the UI build, so `resources.rcc` contains the reCalendar QML.

## 2. `resources.rcc` (Qt 6 `rcc`, after the patch)

Install Qt 6's resource compiler. On Debian or Ubuntu the package is `qt6-base-dev-tools`. The compiler must report Qt 6. Do not use a Qt 5 `rcc`.

From the upstream checkout, run upstream's `scripts/build-ui.sh`:

```sh
/usr/lib/qt6/libexec/rcc --version
RCC=/usr/lib/qt6/libexec/rcc ./scripts/build-ui.sh
```

If `rcc` lives somewhere else, pass that path in `RCC`. The script compiles `packaging/application.qrc` with Qt 6 `rcc` (`--binary --format-version 3 --no-compress`) and writes `dist/remarkable-calendar/resources.rcc`.

Copy that file here:

```sh
cp dist/remarkable-calendar/resources.rcc /path/to/reCalendar/installer/payload/resources.rcc
```

The same script also writes a placeholder icon and copies the manifest into `dist/`. Leave `installer/payload/icon.png` and `installer/payload/manifest.json` as they are in this repository. The manifest here already matches the patched one: the name is reCalendar and the id is unchanged.

## 3. `backend/entry` (static musl build)

Build the upstream Rust backend as a static binary for `aarch64-unknown-linux-musl`.

Upstream's `Cargo.toml` already selects rustls and bundled SQLite:

- `reqwest` is built with default features off and `rustls-tls` on, so the binary does not link OpenSSL.
- `rusqlite` uses the `bundled` feature, so SQLite is compiled into the binary rather than taken from the tablet.

A musl static build also avoids the glibc mismatch that a normal `aarch64-unknown-linux-gnu` cross-compile can hit on the tablet.

Install a current Rust toolchain, [Zig](https://ziglang.org/download/), and [cargo-zigbuild](https://github.com/rust-cross/cargo-zigbuild). From the upstream checkout:

```sh
rustup target add aarch64-unknown-linux-musl
cargo install cargo-zigbuild
cargo zigbuild --release --target aarch64-unknown-linux-musl
mkdir -p /path/to/reCalendar/installer/payload/backend
cp target/aarch64-unknown-linux-musl/release/remarkable-calendar-core \
   /path/to/reCalendar/installer/payload/backend/entry
chmod 755 /path/to/reCalendar/installer/payload/backend/entry
```

`cargo zigbuild --release --target aarch64-unknown-linux-musl` is the release build. Add `--locked` if you want Cargo to use upstream's `Cargo.lock` (that is what upstream's `scripts/build-backend.sh` does for its own target). The crate binary is `remarkable-calendar-core`. AppLoad expects it at `backend/entry`.

Check the result:

```sh
file installer/payload/backend/entry
```

You want an ELF 64-bit ARM aarch64 executable, statically linked against musl, not a dynamically linked glibc binary.

On install, the program copies `manifest.json`, `resources.rcc`, `icon.png` and `backend/entry` to `/home/root/xovi/exthome/appload/remarkable-calendar/` and runs a short self-test of `backend/entry` on the tablet.
