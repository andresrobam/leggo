# leggo

## Install

### macOS (Homebrew)

```bash
brew tap andresrobam/homebrew-tap
brew install leggo
```

### Windows (Scoop)

```powershell
scoop bucket add andresrobam https://github.com/andresrobam/scoop-bucket.git
scoop install andresrobam/leggo
```

## Run

### Installed from scoop/homebrew

```bash
leggo path-to-context-file.yml
```

### From source code

```bash
go mod tidy
go mod vendor
go run . path-to-context-file.yml
```
