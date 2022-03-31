# Nymo WebUI

A simple web UI interface for the [Nymo network core](https://github.com/nymo-net/nymo) implementation.

Frontend is built using Go HTML Template, Bootstrap 5, and native Javascript; Backend using Go built-in webserver and SQLite 3 as database.

This web UI **DOES NOT** encrypt cold data, meaning your private keys and all the messages you received are stored unencrypted on your local device. Use an encrypted filesystem/disk for extra security.

## Usage

Simply download binaries in [Releases](https://github.com/nymo-net/nymo-webui/releases) corresponding to your system and architecture. Run `nymo-webui` to start the program. Make sure `static/`, `view/` are under the working directory of the program.

The default config uses file `./config.toml`. To use another config file, use `-config [path]` command line option. Use `nymo-webui -h` for more information.

Otherwise, see [Compile](#compile) for more information.

## Config

See [`config.toml`](./config.toml) for more information.

## Compile

To build the program, run `go build .` within the source folder.

Building the program requires Go version 1.17+ and since SQLite 3's Go binding is using CGO and `gcc`, C toolchain is needed as well. See [mattn/go-sqlite3#installation](https://github.com/mattn/go-sqlite3#installation) for more information.

## License

All files under this repository are marked with [BSD Zero Clause License](./LICENSE).