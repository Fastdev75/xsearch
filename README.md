# Xsearch

Modern, ultra-fast web content discovery tool written in Go.

```
██╗  ██╗███████╗███████╗ █████╗ ██████╗  ██████╗██╗  ██╗
╚██╗██╔╝██╔════╝██╔════╝██╔══██╗██╔══██╗██╔════╝██║  ██║
 ╚███╔╝ ███████╗█████╗  ███████║██████╔╝██║     ███████║
 ██╔██╗ ╚════██║██╔══╝  ██╔══██║██╔══██╗██║     ██╔══██║
██╔╝ ██╗███████║███████╗██║  ██║██║  ██║╚██████╗██║  ██║
╚═╝  ╚═╝╚══════╝╚══════╝╚═╝  ╚═╝╚═╝  ╚═╝ ╚═════╝╚═╝  ╚═╝
```

## Features

- High-performance concurrent scanning using goroutines
- Real-time colored output
- Automatic SecLists integration (Kali Linux friendly)
- Clean output file with URLs only (no status codes)
- Customizable thread count and timeout
- Extension brute-forcing support
- Status code filtering
- Graceful shutdown with CTRL+C

## Installation

### Method 1: Go Install (Recommended)

```bash
go install -v github.com/projectdiscovery/chaos-client/cmd/chaos@latest
```

### Method 2: Build from Source

```bash
git clone https://github.com/mcauet/xsearch.git
cd xsearch
go build -o xsearch ./cmd/xsearch
sudo mv xsearch /usr/local/bin/
```

## Kali Linux Setup

Xsearch uses SecLists by default. Install it with:

```bash
sudo apt install seclists -y
```

Default wordlist path: `/usr/share/seclists/Discovery/Web-Content/common.txt`

## Usage

### Basic Scan

```bash
xsearch -u https://target.com
```

### With Custom Wordlist

```bash
xsearch -u https://target.com -w /path/to/wordlist.txt
```

### Save Results to File

```bash
xsearch -u https://target.com -o results.txt
```

### With Extensions

```bash
xsearch -u https://target.com -x php,html,js,txt
```

### Filter Status Codes

```bash
xsearch -u https://target.com -status 200,301,403
```

### Full Example

```bash
xsearch -u https://target.com -w wordlist.txt -o results.txt -t 100 -x php,html -timeout 15
```

## Flags

| Flag | Description | Default |
|------|-------------|---------|
| `-u` | Target URL (required) | - |
| `-w` | Wordlist path | SecLists common.txt |
| `-o` | Output file for valid URLs | - |
| `-t` | Number of threads | 50 |
| `-timeout` | HTTP timeout (seconds) | 10 |
| `-status` | Filter by status codes | All except 404 |
| `-x` | File extensions | - |
| `-ua` | Custom User-Agent | Xsearch/1.0 |
| `-silent` | Disable banner | false |
| `-version` | Show version | - |
| `-h` | Show help | - |

## Output

### Terminal Output

Real-time colored output showing status codes and URLs:

```
[200] https://target.com/admin [1.2KB]
[301] https://target.com/login [0B]
[403] https://target.com/.git [287B]
```

### File Output (-o)

Clean list of URLs only (no status codes):

```
https://target.com/admin
https://target.com/login
https://target.com/.git
```

## Legal Disclaimer

Xsearch is intended for authorized security testing and educational purposes only. Users are responsible for ensuring they have proper authorization before scanning any target. Unauthorized access to computer systems is illegal.

## License

MIT License
