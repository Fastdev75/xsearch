package utils

import "fmt"

// ANSI color codes
const (
	Red    = "\033[31m"
	Green  = "\033[32m"
	Yellow = "\033[33m"
	Blue   = "\033[34m"
	Cyan   = "\033[36m"
	White  = "\033[37m"
	Reset  = "\033[0m"
	Bold   = "\033[1m"
)

// Version is set during build or defaults to dev
var Version = "1.0.6"

// Banner displays the Xsearch ASCII art banner in red
func Banner() {
	banner := `
` + Red + Bold + `
██╗  ██╗███████╗███████╗ █████╗ ██████╗  ██████╗██╗  ██╗
╚██╗██╔╝██╔════╝██╔════╝██╔══██╗██╔══██╗██╔════╝██║  ██║
 ╚███╔╝ ███████╗█████╗  ███████║██████╔╝██║     ███████║
 ██╔██╗ ╚════██║██╔══╝  ██╔══██║██╔══██╗██║     ██╔══██║
██╔╝ ██╗███████║███████╗██║  ██║██║  ██║╚██████╗██║  ██║
╚═╝  ╚═╝╚══════╝╚══════╝╚═╝  ╚═╝╚═╝  ╚═╝ ╚═════╝╚═╝  ╚═╝
` + Reset + `
` + Cyan + `        [ Modern Web Content Discovery Tool ]` + Reset + `
` + Yellow + `                    v` + Version + Reset + `
` + White + `           github.com/Fastdev75/xsearch` + Reset + `
`
	fmt.Println(banner)
}

// PrintInfo prints an info message in cyan
func PrintInfo(format string, args ...interface{}) {
	fmt.Printf(Cyan+"[INFO] "+Reset+format+"\n", args...)
}

// PrintSuccess prints a success message in green
func PrintSuccess(format string, args ...interface{}) {
	fmt.Printf(Green+"[+] "+Reset+format+"\n", args...)
}

// PrintWarning prints a warning message in yellow
func PrintWarning(format string, args ...interface{}) {
	fmt.Printf(Yellow+"[!] "+Reset+format+"\n", args...)
}

// PrintError prints an error message in red
func PrintError(format string, args ...interface{}) {
	fmt.Printf(Red+"[-] "+Reset+format+"\n", args...)
}
