# url-downloader

Interactive CLI to clean MP4 URLs and fetch them with `wget`, downloading in parallel where possible.

## Build

```bash
go build -o url-downloader
```

## Run

```bash
./url-downloader -dir ~/Downloads/mobile/ -workers 4
```

Then paste URLs one per line. Use `:go` to start downloading, or `:q` to exit.
