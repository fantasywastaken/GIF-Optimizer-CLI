# GIF-Optimizer-CLI

Command-line GIF optimizer that reduces file size by shrinking the palette, dropping duplicate frames, and resizing while relying only on the Go standard library `image/gif` package.

### How It Works

1. `image/gif` decodes the input into frames, delays, disposal codes, and a global configuration.
2. If `--resize` is provided, each paletted frame is rescaled with nearest-neighbor sampling and the global canvas size is updated.
3. If `--colors` is below 256, a global histogram is built across every frame, the most-used colors form a new palette, and every frame is redrawn with Floyd-Steinberg dithering against that palette.
4. If `--dedup` is enabled, consecutive frames that are visually identical are collapsed and their delays are folded into the preceding frame.
5. `gif.EncodeAll` rewrites the file and a size delta report is printed.

## Setup

### Requirements

- Go 1.21 or newer
- Standard library only

### Installation

```bash
git clone https://github.com/fantasywastaken/GIF-Optimizer-CLI.git
cd GIF-Optimizer-CLI
go build -o gifopt .
```

### Usage

```bash
gifopt input.gif --colors 128 --resize 480x --output out.gif
gifopt banner.gif --colors 64 --dedup=false --output banner.small.gif
gifopt loop.gif --resize x240 --output loop.small.gif
gifopt movie.gif --resize 320x240 --colors 32 --output movie.opt.gif
```

Flags:

| Flag         | Default      | Purpose                                              |
| ------------ | ------------ | ---------------------------------------------------- |
| `--output`   | `output.gif` | Output GIF file                                      |
| `--colors`   | `256`        | Maximum colors in the shared palette (2 to 256)      |
| `--resize`   | (empty)      | Target size `WxH`, `Wx` (width only), or `xH` (height only) |
| `--dedup`    | `true`       | Merge identical consecutive frames                   |

### Features

- Pure standard library, no image codecs beyond `image/gif`.
- Global palette reduction from all frames combined for better color choices.
- Floyd-Steinberg dithering while remapping colors to preserve gradients.
- Nearest-neighbor resize that keeps frame offsets consistent.
- Consecutive frame deduplication that adds skipped delays back to the surviving frame.
- Before-and-after size report with percentage savings.
