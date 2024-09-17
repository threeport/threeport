// +build ignore

package main

import (
	"context"
	_flag "flag"
	_fmt "fmt"
	_ioutil "io/ioutil"
	_log "log"
	"os"
	"os/signal"
	_filepath "path/filepath"
	_sort "sort"
	"strconv"
	_strings "strings"
	"syscall"
	_tabwriter "text/tabwriter"
	"time"
	
)

func main() {
	// Use local types and functions in order to avoid name conflicts with additional magefiles.
	type arguments struct {
		Verbose       bool          // print out log statements
		List          bool          // print out a list of targets
		Help          bool          // print out help for a specific target
		Timeout       time.Duration // set a timeout to running the targets
		Args          []string      // args contain the non-flag command-line arguments
	}

	parseBool := func(env string) bool {
		val := os.Getenv(env)
		if val == "" {
			return false
		}		
		b, err := strconv.ParseBool(val)
		if err != nil {
			_log.Printf("warning: environment variable %s is not a valid bool value: %v", env, val)
			return false
		}
		return b
	}

	parseDuration := func(env string) time.Duration {
		val := os.Getenv(env)
		if val == "" {
			return 0
		}		
		d, err := time.ParseDuration(val)
		if err != nil {
			_log.Printf("warning: environment variable %s is not a valid duration value: %v", env, val)
			return 0
		}
		return d
	}
	args := arguments{}
	fs := _flag.FlagSet{}
	fs.SetOutput(os.Stdout)

	// default flag set with ExitOnError and auto generated PrintDefaults should be sufficient
	fs.BoolVar(&args.Verbose, "v", parseBool("MAGEFILE_VERBOSE"), "show verbose output when running targets")
	fs.BoolVar(&args.List, "l", parseBool("MAGEFILE_LIST"), "list targets for this binary")
	fs.BoolVar(&args.Help, "h", parseBool("MAGEFILE_HELP"), "print out help for a specific target")
	fs.DurationVar(&args.Timeout, "t", parseDuration("MAGEFILE_TIMEOUT"), "timeout in duration parsable format (e.g. 5m30s)")
	fs.Usage = func() {
		_fmt.Fprintf(os.Stdout, `
%s [options] [target]

Commands:
  -l    list targets in this binary
  -h    show this help

Options:
  -h    show description of a target
  -t <string>
        timeout in duration parsable format (e.g. 5m30s)
  -v    show verbose output when running targets
 `[1:], _filepath.Base(os.Args[0]))
	}
	if err := fs.Parse(os.Args[1:]); err != nil {
		// flag will have printed out an error already.
		return
	}
	args.Args = fs.Args()
	if args.Help && len(args.Args) == 0 {
		fs.Usage()
		return
	}
		
	// color is ANSI color type
	type color int

	// If you add/change/remove any items in this constant,
	// you will need to run "stringer -type=color" in this directory again.
	// NOTE: Please keep the list in an alphabetical order.
	const (
		black color = iota
		red
		green
		yellow
		blue
		magenta
		cyan
		white
		brightblack
		brightred
		brightgreen
		brightyellow
		brightblue
		brightmagenta
		brightcyan
		brightwhite
	)

	// AnsiColor are ANSI color codes for supported terminal colors.
	var ansiColor = map[color]string{
		black:         "\u001b[30m",
		red:           "\u001b[31m",
		green:         "\u001b[32m",
		yellow:        "\u001b[33m",
		blue:          "\u001b[34m",
		magenta:       "\u001b[35m",
		cyan:          "\u001b[36m",
		white:         "\u001b[37m",
		brightblack:   "\u001b[30;1m",
		brightred:     "\u001b[31;1m",
		brightgreen:   "\u001b[32;1m",
		brightyellow:  "\u001b[33;1m",
		brightblue:    "\u001b[34;1m",
		brightmagenta: "\u001b[35;1m",
		brightcyan:    "\u001b[36;1m",
		brightwhite:   "\u001b[37;1m",
	}
	
	const _color_name = "blackredgreenyellowbluemagentacyanwhitebrightblackbrightredbrightgreenbrightyellowbrightbluebrightmagentabrightcyanbrightwhite"

	var _color_index = [...]uint8{0, 5, 8, 13, 19, 23, 30, 34, 39, 50, 59, 70, 82, 92, 105, 115, 126}

	colorToLowerString := func (i color) string {
		if i < 0 || i >= color(len(_color_index)-1) {
			return "color(" + strconv.FormatInt(int64(i), 10) + ")"
		}
		return _color_name[_color_index[i]:_color_index[i+1]]
	}

	// ansiColorReset is an ANSI color code to reset the terminal color.
	const ansiColorReset = "\033[0m"

	// defaultTargetAnsiColor is a default ANSI color for colorizing targets.
	// It is set to Cyan as an arbitrary color, because it has a neutral meaning
	var defaultTargetAnsiColor = ansiColor[cyan]

	getAnsiColor := func(color string) (string, bool) {
		colorLower := _strings.ToLower(color)
		for k, v := range ansiColor {
			colorConstLower := colorToLowerString(k)
			if colorConstLower == colorLower {
				return v, true
			}
		}
		return "", false
	}

	// Terminals which  don't support color:
	// 	TERM=vt100
	// 	TERM=cygwin
	// 	TERM=xterm-mono
    var noColorTerms = map[string]bool{
		"vt100":      false,
		"cygwin":     false,
		"xterm-mono": false,
	}

	// terminalSupportsColor checks if the current console supports color output
	//
	// Supported:
	// 	linux, mac, or windows's ConEmu, Cmder, putty, git-bash.exe, pwsh.exe
	// Not supported:
	// 	windows cmd.exe, powerShell.exe
	terminalSupportsColor := func() bool {
		envTerm := os.Getenv("TERM")
		if _, ok := noColorTerms[envTerm]; ok {
			return false
		}
		return true
	}

	// enableColor reports whether the user has requested to enable a color output.
	enableColor := func() bool {
		b, _ := strconv.ParseBool(os.Getenv("MAGEFILE_ENABLE_COLOR"))
		return b
	}

	// targetColor returns the ANSI color which should be used to colorize targets.
	targetColor := func() string {
		s, exists := os.LookupEnv("MAGEFILE_TARGET_COLOR")
		if exists == true {
			if c, ok := getAnsiColor(s); ok == true {
				return c
			}
		}
		return defaultTargetAnsiColor
	}

	// store the color terminal variables, so that the detection isn't repeated for each target
	var enableColorValue = enableColor() && terminalSupportsColor()
	var targetColorValue = targetColor()

	printName := func(str string) string {
		if enableColorValue {
			return _fmt.Sprintf("%s%s%s", targetColorValue, str, ansiColorReset)
		} else {
			return str
		}
	}

	list := func() error {
		
		targets := map[string]string{
			"automatedTests": "runs automated tests.",
			"buildAgent": "builds the binary for the agent.",
			"buildAgentImage": "builds and pushes the agent image.",
			"buildAll": "builds the binaries for all components.",
			"buildAllImages": "builds and pushes images for all components.",
			"buildApi": "builds the REST API binary.",
			"buildApiImage": "builds and pushes the REST API image.",
			"buildAwsController": "builds the binary for the aws-controller.",
			"buildAwsControllerImage": "builds and pushes the container image for the aws-controller.",
			"buildControlPlaneController": "builds the binary for the control-plane-controller.",
			"buildControlPlaneControllerImage": "builds and pushes the container image for the control-plane-controller.",
			"buildDatabaseMigrator": "builds the binary for the database-migrator.",
			"buildDatabaseMigratorImage": "builds and pushes the database-migrator image.",
			"buildGatewayController": "builds the binary for the gateway-controller.",
			"buildGatewayControllerImage": "builds and pushes the container image for the gateway-controller.",
			"buildHelmWorkloadController": "builds the binary for the helm-workload-controller.",
			"buildHelmWorkloadControllerImage": "builds and pushes the container image for the helm-workload-controller.",
			"buildImage": "builds a container image for a Threeport control plane component for the given architecture.",
			"buildKubernetesRuntimeController": "builds the binary for the kubernetes-runtime-controller.",
			"buildKubernetesRuntimeControllerImage": "builds and pushes the container image for the kubernetes-runtime-controller.",
			"buildObservabilityController": "builds the binary for the observability-controller.",
			"buildObservabilityControllerImage": "builds and pushes the container image for the observability-controller.",
			"buildSecretController": "builds the binary for the secret-controller.",
			"buildSecretControllerImage": "builds and pushes the container image for the secret-controller.",
			"buildTerraformController": "builds the binary for the terraform-controller.",
			"buildTerraformControllerImage": "builds and pushes the container image for the terraform-controller.",
			"buildTptctl": "builds tptctl binary.",
			"buildTptdev": "builds tptdev binary.",
			"buildWorkloadController": "builds the binary for the workload-controller.",
			"buildWorkloadControllerImage": "builds and pushes the container image for the workload-controller.",
			"cleanLocalRegistry": "stops and removes the local container registry.",
			"createLocalRegistry": "starts a docker container to serve as a local container registry.",
			"devDown": "deletes the local development environment.",
			"devForwardAPI": "forwards local port 1323 to the local dev API",
			"devForwardCrdb": "forwards local port 26257 to local dev cockroach database",
			"devForwardNats": "forwards local port 33993 to the local dev API nats server",
			"devImage": "builds and pushes a container image using the alpine Dockerfile.",
			"devUp": "runs a local development environment.",
			"docs": "",
			"e2e": "calls ginkgo to run the e2e tests suite.",
			"e2eClean": "removes the kind cluster and local container registry for e2e testing.",
			"e2eLocal": "is a wrapper for E2e that uses kind, a local image repo in a docker container and cleans up at completion.",
			"generate": "runs code generation.",
			"generateCode": "generates code with threeport-sdk.",
			"generateDocs": "generates API swagger docs.",
			"installSdk": "builds SDK binary and installs in GOPATH.",
			"installTptctl": "installs tptctl binary at /usr/local/bin/.",
			"installTptdev": "installs tptdev binary at /usr/local/bin/.",
			"integration": "runs integration tests against an existing Threeport control plane.",
			"testCommits": "checks to make sure commit messages follow conventional commits format.",
		}

		keys := make([]string, 0, len(targets))
		for name := range targets {
			keys = append(keys, name)
		}
		_sort.Strings(keys)

		_fmt.Println("Targets:")
		w := _tabwriter.NewWriter(os.Stdout, 0, 4, 4, ' ', 0)
		for _, name := range keys {
			_fmt.Fprintf(w, "  %v\t%v\n", printName(name), targets[name])
		}
		err := w.Flush()
		return err
	}

	var ctx context.Context
	ctxCancel := func(){}

	// by deferring in a closure, we let the cancel function get replaced
	// by the getContext function.
	defer func() {
		ctxCancel()
	}()

	getContext := func() (context.Context, func()) {
		if ctx == nil {
			if args.Timeout != 0 {
				ctx, ctxCancel = context.WithTimeout(context.Background(), args.Timeout)
			} else {
				ctx, ctxCancel = context.WithCancel(context.Background())
			}
		}

		return ctx, ctxCancel
	}

	runTarget := func(logger *_log.Logger, fn func(context.Context) error) interface{} {
		var err interface{}
		ctx, cancel := getContext()
		d := make(chan interface{})
		go func() {
			defer func() {
				err := recover()
				d <- err
			}()
			err := fn(ctx)
			d <- err
		}()
		sigCh := make(chan os.Signal, 1)
		signal.Notify(sigCh, syscall.SIGINT)
		select {
		case <-sigCh:
			logger.Println("cancelling mage targets, waiting up to 5 seconds for cleanup...")
			cancel()
			cleanupCh := time.After(5 * time.Second)

			select {
			// target exited by itself
			case err = <-d:
				return err
			// cleanup timeout exceeded
			case <-cleanupCh:
				return _fmt.Errorf("cleanup timeout exceeded")
			// second SIGINT received
			case <-sigCh:
				logger.Println("exiting mage")
				return _fmt.Errorf("exit forced")
			}
		case <-ctx.Done():
			cancel()
			e := ctx.Err()
			_fmt.Printf("ctx err: %v\n", e)
			return e
		case err = <-d:
			// we intentionally don't cancel the context here, because
			// the next target will need to run with the same context.
			return err
		}
	}
	// This is necessary in case there aren't any targets, to avoid an unused
	// variable error.
	_ = runTarget

	handleError := func(logger *_log.Logger, err interface{}) {
		if err != nil {
			logger.Printf("Error: %+v\n", err)
			type code interface {
				ExitStatus() int
			}
			if c, ok := err.(code); ok {
				os.Exit(c.ExitStatus())
			}
			os.Exit(1)
		}
	}
	_ = handleError

	// Set MAGEFILE_VERBOSE so mg.Verbose() reflects the flag value.
	if args.Verbose {
		os.Setenv("MAGEFILE_VERBOSE", "1")
	} else {
		os.Setenv("MAGEFILE_VERBOSE", "0")
	}

	_log.SetFlags(0)
	if !args.Verbose {
		_log.SetOutput(_ioutil.Discard)
	}
	logger := _log.New(os.Stderr, "", 0)
	if args.List {
		if err := list(); err != nil {
			_log.Println(err)
			os.Exit(1)
		}
		return
	}

	if args.Help {
		if len(args.Args) < 1 {
			logger.Println("no target specified")
			os.Exit(2)
		}
		switch _strings.ToLower(args.Args[0]) {
			case "automatedtests":
				_fmt.Println("AutomatedTests runs automated tests.")
				_fmt.Println()
				
				_fmt.Print("Usage:\n\n\tmage automatedtests\n\n")
				var aliases []string
				if len(aliases) > 0 {
					_fmt.Printf("Aliases: %s\n\n", _strings.Join(aliases, ", "))
				}
				return
			case "buildagent":
				_fmt.Println("BuildAgent builds the binary for the agent.")
				_fmt.Println()
				
				_fmt.Print("Usage:\n\n\tmage buildagent\n\n")
				var aliases []string
				if len(aliases) > 0 {
					_fmt.Printf("Aliases: %s\n\n", _strings.Join(aliases, ", "))
				}
				return
			case "buildagentimage":
				_fmt.Println("BuildAgentImage builds and pushes the agent image.")
				_fmt.Println()
				
				_fmt.Print("Usage:\n\n\tmage buildagentimage\n\n")
				var aliases []string
				if len(aliases) > 0 {
					_fmt.Printf("Aliases: %s\n\n", _strings.Join(aliases, ", "))
				}
				return
			case "buildall":
				_fmt.Println("BuildAll builds the binaries for all components.")
				_fmt.Println()
				
				_fmt.Print("Usage:\n\n\tmage buildall\n\n")
				var aliases []string
				if len(aliases) > 0 {
					_fmt.Printf("Aliases: %s\n\n", _strings.Join(aliases, ", "))
				}
				return
			case "buildallimages":
				_fmt.Println("BuildAllImages builds and pushes images for all components.")
				_fmt.Println()
				
				_fmt.Print("Usage:\n\n\tmage buildallimages\n\n")
				var aliases []string
				if len(aliases) > 0 {
					_fmt.Printf("Aliases: %s\n\n", _strings.Join(aliases, ", "))
				}
				return
			case "buildapi":
				_fmt.Println("BuildApi builds the REST API binary.")
				_fmt.Println()
				
				_fmt.Print("Usage:\n\n\tmage buildapi\n\n")
				var aliases []string
				if len(aliases) > 0 {
					_fmt.Printf("Aliases: %s\n\n", _strings.Join(aliases, ", "))
				}
				return
			case "buildapiimage":
				_fmt.Println("BuildApiImage builds and pushes the REST API image.")
				_fmt.Println()
				
				_fmt.Print("Usage:\n\n\tmage buildapiimage\n\n")
				var aliases []string
				if len(aliases) > 0 {
					_fmt.Printf("Aliases: %s\n\n", _strings.Join(aliases, ", "))
				}
				return
			case "buildawscontroller":
				_fmt.Println("BuildAwsController builds the binary for the aws-controller.")
				_fmt.Println()
				
				_fmt.Print("Usage:\n\n\tmage buildawscontroller\n\n")
				var aliases []string
				if len(aliases) > 0 {
					_fmt.Printf("Aliases: %s\n\n", _strings.Join(aliases, ", "))
				}
				return
			case "buildawscontrollerimage":
				_fmt.Println("BuildAwsControllerImage builds and pushes the container image for the aws-controller.")
				_fmt.Println()
				
				_fmt.Print("Usage:\n\n\tmage buildawscontrollerimage\n\n")
				var aliases []string
				if len(aliases) > 0 {
					_fmt.Printf("Aliases: %s\n\n", _strings.Join(aliases, ", "))
				}
				return
			case "buildcontrolplanecontroller":
				_fmt.Println("BuildControlPlaneController builds the binary for the control-plane-controller.")
				_fmt.Println()
				
				_fmt.Print("Usage:\n\n\tmage buildcontrolplanecontroller\n\n")
				var aliases []string
				if len(aliases) > 0 {
					_fmt.Printf("Aliases: %s\n\n", _strings.Join(aliases, ", "))
				}
				return
			case "buildcontrolplanecontrollerimage":
				_fmt.Println("BuildControlPlaneControllerImage builds and pushes the container image for the control-plane-controller.")
				_fmt.Println()
				
				_fmt.Print("Usage:\n\n\tmage buildcontrolplanecontrollerimage\n\n")
				var aliases []string
				if len(aliases) > 0 {
					_fmt.Printf("Aliases: %s\n\n", _strings.Join(aliases, ", "))
				}
				return
			case "builddatabasemigrator":
				_fmt.Println("BuildDatabaseMigrator builds the binary for the database-migrator.")
				_fmt.Println()
				
				_fmt.Print("Usage:\n\n\tmage builddatabasemigrator\n\n")
				var aliases []string
				if len(aliases) > 0 {
					_fmt.Printf("Aliases: %s\n\n", _strings.Join(aliases, ", "))
				}
				return
			case "builddatabasemigratorimage":
				_fmt.Println("BuildDatabaseMigratorImage builds and pushes the database-migrator image.")
				_fmt.Println()
				
				_fmt.Print("Usage:\n\n\tmage builddatabasemigratorimage\n\n")
				var aliases []string
				if len(aliases) > 0 {
					_fmt.Printf("Aliases: %s\n\n", _strings.Join(aliases, ", "))
				}
				return
			case "buildgatewaycontroller":
				_fmt.Println("BuildGatewayController builds the binary for the gateway-controller.")
				_fmt.Println()
				
				_fmt.Print("Usage:\n\n\tmage buildgatewaycontroller\n\n")
				var aliases []string
				if len(aliases) > 0 {
					_fmt.Printf("Aliases: %s\n\n", _strings.Join(aliases, ", "))
				}
				return
			case "buildgatewaycontrollerimage":
				_fmt.Println("BuildGatewayControllerImage builds and pushes the container image for the gateway-controller.")
				_fmt.Println()
				
				_fmt.Print("Usage:\n\n\tmage buildgatewaycontrollerimage\n\n")
				var aliases []string
				if len(aliases) > 0 {
					_fmt.Printf("Aliases: %s\n\n", _strings.Join(aliases, ", "))
				}
				return
			case "buildhelmworkloadcontroller":
				_fmt.Println("BuildHelmWorkloadController builds the binary for the helm-workload-controller.")
				_fmt.Println()
				
				_fmt.Print("Usage:\n\n\tmage buildhelmworkloadcontroller\n\n")
				var aliases []string
				if len(aliases) > 0 {
					_fmt.Printf("Aliases: %s\n\n", _strings.Join(aliases, ", "))
				}
				return
			case "buildhelmworkloadcontrollerimage":
				_fmt.Println("BuildHelmWorkloadControllerImage builds and pushes the container image for the helm-workload-controller.")
				_fmt.Println()
				
				_fmt.Print("Usage:\n\n\tmage buildhelmworkloadcontrollerimage\n\n")
				var aliases []string
				if len(aliases) > 0 {
					_fmt.Printf("Aliases: %s\n\n", _strings.Join(aliases, ", "))
				}
				return
			case "buildimage":
				_fmt.Println("BuildImage builds a container image for a Threeport control plane component for the given architecture.")
				_fmt.Println()
				
				_fmt.Print("Usage:\n\n\tmage buildimage <component> <imageRepo> <imageTag> <pushImage> <loadImage> <arch>\n\n")
				var aliases []string
				if len(aliases) > 0 {
					_fmt.Printf("Aliases: %s\n\n", _strings.Join(aliases, ", "))
				}
				return
			case "buildkubernetesruntimecontroller":
				_fmt.Println("BuildKubernetesRuntimeController builds the binary for the kubernetes-runtime-controller.")
				_fmt.Println()
				
				_fmt.Print("Usage:\n\n\tmage buildkubernetesruntimecontroller\n\n")
				var aliases []string
				if len(aliases) > 0 {
					_fmt.Printf("Aliases: %s\n\n", _strings.Join(aliases, ", "))
				}
				return
			case "buildkubernetesruntimecontrollerimage":
				_fmt.Println("BuildKubernetesRuntimeControllerImage builds and pushes the container image for the kubernetes-runtime-controller.")
				_fmt.Println()
				
				_fmt.Print("Usage:\n\n\tmage buildkubernetesruntimecontrollerimage\n\n")
				var aliases []string
				if len(aliases) > 0 {
					_fmt.Printf("Aliases: %s\n\n", _strings.Join(aliases, ", "))
				}
				return
			case "buildobservabilitycontroller":
				_fmt.Println("BuildObservabilityController builds the binary for the observability-controller.")
				_fmt.Println()
				
				_fmt.Print("Usage:\n\n\tmage buildobservabilitycontroller\n\n")
				var aliases []string
				if len(aliases) > 0 {
					_fmt.Printf("Aliases: %s\n\n", _strings.Join(aliases, ", "))
				}
				return
			case "buildobservabilitycontrollerimage":
				_fmt.Println("BuildObservabilityControllerImage builds and pushes the container image for the observability-controller.")
				_fmt.Println()
				
				_fmt.Print("Usage:\n\n\tmage buildobservabilitycontrollerimage\n\n")
				var aliases []string
				if len(aliases) > 0 {
					_fmt.Printf("Aliases: %s\n\n", _strings.Join(aliases, ", "))
				}
				return
			case "buildsecretcontroller":
				_fmt.Println("BuildSecretController builds the binary for the secret-controller.")
				_fmt.Println()
				
				_fmt.Print("Usage:\n\n\tmage buildsecretcontroller\n\n")
				var aliases []string
				if len(aliases) > 0 {
					_fmt.Printf("Aliases: %s\n\n", _strings.Join(aliases, ", "))
				}
				return
			case "buildsecretcontrollerimage":
				_fmt.Println("BuildSecretControllerImage builds and pushes the container image for the secret-controller.")
				_fmt.Println()
				
				_fmt.Print("Usage:\n\n\tmage buildsecretcontrollerimage\n\n")
				var aliases []string
				if len(aliases) > 0 {
					_fmt.Printf("Aliases: %s\n\n", _strings.Join(aliases, ", "))
				}
				return
			case "buildterraformcontroller":
				_fmt.Println("BuildTerraformController builds the binary for the terraform-controller.")
				_fmt.Println()
				
				_fmt.Print("Usage:\n\n\tmage buildterraformcontroller\n\n")
				var aliases []string
				if len(aliases) > 0 {
					_fmt.Printf("Aliases: %s\n\n", _strings.Join(aliases, ", "))
				}
				return
			case "buildterraformcontrollerimage":
				_fmt.Println("BuildTerraformControllerImage builds and pushes the container image for the terraform-controller.")
				_fmt.Println()
				
				_fmt.Print("Usage:\n\n\tmage buildterraformcontrollerimage\n\n")
				var aliases []string
				if len(aliases) > 0 {
					_fmt.Printf("Aliases: %s\n\n", _strings.Join(aliases, ", "))
				}
				return
			case "buildtptctl":
				_fmt.Println("BuildTptctl builds tptctl binary.")
				_fmt.Println()
				
				_fmt.Print("Usage:\n\n\tmage buildtptctl\n\n")
				var aliases []string
				if len(aliases) > 0 {
					_fmt.Printf("Aliases: %s\n\n", _strings.Join(aliases, ", "))
				}
				return
			case "buildtptdev":
				_fmt.Println("BuildTptdev builds tptdev binary.")
				_fmt.Println()
				
				_fmt.Print("Usage:\n\n\tmage buildtptdev\n\n")
				var aliases []string
				if len(aliases) > 0 {
					_fmt.Printf("Aliases: %s\n\n", _strings.Join(aliases, ", "))
				}
				return
			case "buildworkloadcontroller":
				_fmt.Println("BuildWorkloadController builds the binary for the workload-controller.")
				_fmt.Println()
				
				_fmt.Print("Usage:\n\n\tmage buildworkloadcontroller\n\n")
				var aliases []string
				if len(aliases) > 0 {
					_fmt.Printf("Aliases: %s\n\n", _strings.Join(aliases, ", "))
				}
				return
			case "buildworkloadcontrollerimage":
				_fmt.Println("BuildWorkloadControllerImage builds and pushes the container image for the workload-controller.")
				_fmt.Println()
				
				_fmt.Print("Usage:\n\n\tmage buildworkloadcontrollerimage\n\n")
				var aliases []string
				if len(aliases) > 0 {
					_fmt.Printf("Aliases: %s\n\n", _strings.Join(aliases, ", "))
				}
				return
			case "cleanlocalregistry":
				_fmt.Println("CleanLocalRegistry stops and removes the local container registry.")
				_fmt.Println()
				
				_fmt.Print("Usage:\n\n\tmage cleanlocalregistry\n\n")
				var aliases []string
				if len(aliases) > 0 {
					_fmt.Printf("Aliases: %s\n\n", _strings.Join(aliases, ", "))
				}
				return
			case "createlocalregistry":
				_fmt.Println("CreateLocalRegistry starts a docker container to serve as a local container registry.")
				_fmt.Println()
				
				_fmt.Print("Usage:\n\n\tmage createlocalregistry\n\n")
				var aliases []string
				if len(aliases) > 0 {
					_fmt.Printf("Aliases: %s\n\n", _strings.Join(aliases, ", "))
				}
				return
			case "devdown":
				_fmt.Println("DevDown deletes the local development environment.")
				_fmt.Println()
				
				_fmt.Print("Usage:\n\n\tmage devdown\n\n")
				var aliases []string
				if len(aliases) > 0 {
					_fmt.Printf("Aliases: %s\n\n", _strings.Join(aliases, ", "))
				}
				return
			case "devforwardapi":
				_fmt.Println("DevForwardAPI forwards local port 1323 to the local dev API")
				_fmt.Println()
				
				_fmt.Print("Usage:\n\n\tmage devforwardapi\n\n")
				var aliases []string
				if len(aliases) > 0 {
					_fmt.Printf("Aliases: %s\n\n", _strings.Join(aliases, ", "))
				}
				return
			case "devforwardcrdb":
				_fmt.Println("DevForwardCrdb forwards local port 26257 to local dev cockroach database")
				_fmt.Println()
				
				_fmt.Print("Usage:\n\n\tmage devforwardcrdb\n\n")
				var aliases []string
				if len(aliases) > 0 {
					_fmt.Printf("Aliases: %s\n\n", _strings.Join(aliases, ", "))
				}
				return
			case "devforwardnats":
				_fmt.Println("DevForwardNats forwards local port 33993 to the local dev API nats server")
				_fmt.Println()
				
				_fmt.Print("Usage:\n\n\tmage devforwardnats\n\n")
				var aliases []string
				if len(aliases) > 0 {
					_fmt.Printf("Aliases: %s\n\n", _strings.Join(aliases, ", "))
				}
				return
			case "devimage":
				_fmt.Println("DevImage builds and pushes a container image using the alpine Dockerfile.")
				_fmt.Println()
				
				_fmt.Print("Usage:\n\n\tmage devimage <component> <imageRepo> <imageName> <imageTag> <pushImage> <loadImage>\n\n")
				var aliases []string
				if len(aliases) > 0 {
					_fmt.Printf("Aliases: %s\n\n", _strings.Join(aliases, ", "))
				}
				return
			case "devup":
				_fmt.Println("DevUp runs a local development environment.")
				_fmt.Println()
				
				_fmt.Print("Usage:\n\n\tmage devup\n\n")
				var aliases []string
				if len(aliases) > 0 {
					_fmt.Printf("Aliases: %s\n\n", _strings.Join(aliases, ", "))
				}
				return
			case "docs":
				
				_fmt.Print("Usage:\n\n\tmage docs\n\n")
				var aliases []string
				if len(aliases) > 0 {
					_fmt.Printf("Aliases: %s\n\n", _strings.Join(aliases, ", "))
				}
				return
			case "e2e":
				_fmt.Println("E2e calls ginkgo to run the e2e tests suite.")
				_fmt.Println()
				
				_fmt.Print("Usage:\n\n\tmage e2e <provider> <imageRepo> <clean>\n\n")
				var aliases []string
				if len(aliases) > 0 {
					_fmt.Printf("Aliases: %s\n\n", _strings.Join(aliases, ", "))
				}
				return
			case "e2eclean":
				_fmt.Println("E2eClean removes the kind cluster and local container registry for e2e testing.")
				_fmt.Println()
				
				_fmt.Print("Usage:\n\n\tmage e2eclean\n\n")
				var aliases []string
				if len(aliases) > 0 {
					_fmt.Printf("Aliases: %s\n\n", _strings.Join(aliases, ", "))
				}
				return
			case "e2elocal":
				_fmt.Println("E2eLocal is a wrapper for E2e that uses kind, a local image repo in a docker container and cleans up at completion.")
				_fmt.Println()
				
				_fmt.Print("Usage:\n\n\tmage e2elocal\n\n")
				var aliases []string
				if len(aliases) > 0 {
					_fmt.Printf("Aliases: %s\n\n", _strings.Join(aliases, ", "))
				}
				return
			case "generate":
				_fmt.Println("Generate runs code generation.  It runs threeport-sdk and generates API swagger docs.")
				_fmt.Println()
				
				_fmt.Print("Usage:\n\n\tmage generate\n\n")
				var aliases []string
				if len(aliases) > 0 {
					_fmt.Printf("Aliases: %s\n\n", _strings.Join(aliases, ", "))
				}
				return
			case "generatecode":
				_fmt.Println("GenerateCode generates code with threeport-sdk.")
				_fmt.Println()
				
				_fmt.Print("Usage:\n\n\tmage generatecode\n\n")
				var aliases []string
				if len(aliases) > 0 {
					_fmt.Printf("Aliases: %s\n\n", _strings.Join(aliases, ", "))
				}
				return
			case "generatedocs":
				_fmt.Println("GenerateDocs generates API swagger docs.")
				_fmt.Println()
				
				_fmt.Print("Usage:\n\n\tmage generatedocs\n\n")
				var aliases []string
				if len(aliases) > 0 {
					_fmt.Printf("Aliases: %s\n\n", _strings.Join(aliases, ", "))
				}
				return
			case "installsdk":
				_fmt.Println("InstallSdk builds SDK binary and installs in GOPATH.")
				_fmt.Println()
				
				_fmt.Print("Usage:\n\n\tmage installsdk\n\n")
				var aliases []string
				if len(aliases) > 0 {
					_fmt.Printf("Aliases: %s\n\n", _strings.Join(aliases, ", "))
				}
				return
			case "installtptctl":
				_fmt.Println("InstallTptctl installs tptctl binary at /usr/local/bin/.")
				_fmt.Println()
				
				_fmt.Print("Usage:\n\n\tmage installtptctl\n\n")
				var aliases []string
				if len(aliases) > 0 {
					_fmt.Printf("Aliases: %s\n\n", _strings.Join(aliases, ", "))
				}
				return
			case "installtptdev":
				_fmt.Println("InstallTptdev installs tptdev binary at /usr/local/bin/.")
				_fmt.Println()
				
				_fmt.Print("Usage:\n\n\tmage installtptdev\n\n")
				var aliases []string
				if len(aliases) > 0 {
					_fmt.Printf("Aliases: %s\n\n", _strings.Join(aliases, ", "))
				}
				return
			case "integration":
				_fmt.Println("Integration runs integration tests against an existing Threeport control plane.")
				_fmt.Println()
				
				_fmt.Print("Usage:\n\n\tmage integration\n\n")
				var aliases []string
				if len(aliases) > 0 {
					_fmt.Printf("Aliases: %s\n\n", _strings.Join(aliases, ", "))
				}
				return
			case "testcommits":
				_fmt.Println("TestCommits checks to make sure commit messages follow conventional commits format.")
				_fmt.Println()
				
				_fmt.Print("Usage:\n\n\tmage testcommits\n\n")
				var aliases []string
				if len(aliases) > 0 {
					_fmt.Printf("Aliases: %s\n\n", _strings.Join(aliases, ", "))
				}
				return
			default:
				logger.Printf("Unknown target: %q\n", args.Args[0])
				os.Exit(2)
		}
	}
	if len(args.Args) < 1 {
		if err := list(); err != nil {
			logger.Println("Error:", err)
			os.Exit(1)
		}
		return
	}
	for x := 0; x < len(args.Args); {
		target := args.Args[x]
		x++

		// resolve aliases
		switch _strings.ToLower(target) {
		
		}

		switch _strings.ToLower(target) {
		
			case "automatedtests":
				expected := x + 0
				if expected > len(args.Args) {
					// note that expected and args at this point include the arg for the target itself
					// so we subtract 1 here to show the number of args without the target.
					logger.Printf("not enough arguments for target \"AutomatedTests\", expected %v, got %v\n", expected-1, len(args.Args)-1)
					os.Exit(2)
				}
				if args.Verbose {
					logger.Println("Running target:", "AutomatedTests")
				}
				
				wrapFn := func(ctx context.Context) error {
					return AutomatedTests()
				}
				ret := runTarget(logger, wrapFn)
				handleError(logger, ret)
			case "buildagent":
				expected := x + 0
				if expected > len(args.Args) {
					// note that expected and args at this point include the arg for the target itself
					// so we subtract 1 here to show the number of args without the target.
					logger.Printf("not enough arguments for target \"BuildAgent\", expected %v, got %v\n", expected-1, len(args.Args)-1)
					os.Exit(2)
				}
				if args.Verbose {
					logger.Println("Running target:", "BuildAgent")
				}
				
				wrapFn := func(ctx context.Context) error {
					return BuildAgent()
				}
				ret := runTarget(logger, wrapFn)
				handleError(logger, ret)
			case "buildagentimage":
				expected := x + 0
				if expected > len(args.Args) {
					// note that expected and args at this point include the arg for the target itself
					// so we subtract 1 here to show the number of args without the target.
					logger.Printf("not enough arguments for target \"BuildAgentImage\", expected %v, got %v\n", expected-1, len(args.Args)-1)
					os.Exit(2)
				}
				if args.Verbose {
					logger.Println("Running target:", "BuildAgentImage")
				}
				
				wrapFn := func(ctx context.Context) error {
					return BuildAgentImage()
				}
				ret := runTarget(logger, wrapFn)
				handleError(logger, ret)
			case "buildall":
				expected := x + 0
				if expected > len(args.Args) {
					// note that expected and args at this point include the arg for the target itself
					// so we subtract 1 here to show the number of args without the target.
					logger.Printf("not enough arguments for target \"BuildAll\", expected %v, got %v\n", expected-1, len(args.Args)-1)
					os.Exit(2)
				}
				if args.Verbose {
					logger.Println("Running target:", "BuildAll")
				}
				
				wrapFn := func(ctx context.Context) error {
					return BuildAll()
				}
				ret := runTarget(logger, wrapFn)
				handleError(logger, ret)
			case "buildallimages":
				expected := x + 0
				if expected > len(args.Args) {
					// note that expected and args at this point include the arg for the target itself
					// so we subtract 1 here to show the number of args without the target.
					logger.Printf("not enough arguments for target \"BuildAllImages\", expected %v, got %v\n", expected-1, len(args.Args)-1)
					os.Exit(2)
				}
				if args.Verbose {
					logger.Println("Running target:", "BuildAllImages")
				}
				
				wrapFn := func(ctx context.Context) error {
					return BuildAllImages()
				}
				ret := runTarget(logger, wrapFn)
				handleError(logger, ret)
			case "buildapi":
				expected := x + 0
				if expected > len(args.Args) {
					// note that expected and args at this point include the arg for the target itself
					// so we subtract 1 here to show the number of args without the target.
					logger.Printf("not enough arguments for target \"BuildApi\", expected %v, got %v\n", expected-1, len(args.Args)-1)
					os.Exit(2)
				}
				if args.Verbose {
					logger.Println("Running target:", "BuildApi")
				}
				
				wrapFn := func(ctx context.Context) error {
					return BuildApi()
				}
				ret := runTarget(logger, wrapFn)
				handleError(logger, ret)
			case "buildapiimage":
				expected := x + 0
				if expected > len(args.Args) {
					// note that expected and args at this point include the arg for the target itself
					// so we subtract 1 here to show the number of args without the target.
					logger.Printf("not enough arguments for target \"BuildApiImage\", expected %v, got %v\n", expected-1, len(args.Args)-1)
					os.Exit(2)
				}
				if args.Verbose {
					logger.Println("Running target:", "BuildApiImage")
				}
				
				wrapFn := func(ctx context.Context) error {
					return BuildApiImage()
				}
				ret := runTarget(logger, wrapFn)
				handleError(logger, ret)
			case "buildawscontroller":
				expected := x + 0
				if expected > len(args.Args) {
					// note that expected and args at this point include the arg for the target itself
					// so we subtract 1 here to show the number of args without the target.
					logger.Printf("not enough arguments for target \"BuildAwsController\", expected %v, got %v\n", expected-1, len(args.Args)-1)
					os.Exit(2)
				}
				if args.Verbose {
					logger.Println("Running target:", "BuildAwsController")
				}
				
				wrapFn := func(ctx context.Context) error {
					return BuildAwsController()
				}
				ret := runTarget(logger, wrapFn)
				handleError(logger, ret)
			case "buildawscontrollerimage":
				expected := x + 0
				if expected > len(args.Args) {
					// note that expected and args at this point include the arg for the target itself
					// so we subtract 1 here to show the number of args without the target.
					logger.Printf("not enough arguments for target \"BuildAwsControllerImage\", expected %v, got %v\n", expected-1, len(args.Args)-1)
					os.Exit(2)
				}
				if args.Verbose {
					logger.Println("Running target:", "BuildAwsControllerImage")
				}
				
				wrapFn := func(ctx context.Context) error {
					return BuildAwsControllerImage()
				}
				ret := runTarget(logger, wrapFn)
				handleError(logger, ret)
			case "buildcontrolplanecontroller":
				expected := x + 0
				if expected > len(args.Args) {
					// note that expected and args at this point include the arg for the target itself
					// so we subtract 1 here to show the number of args without the target.
					logger.Printf("not enough arguments for target \"BuildControlPlaneController\", expected %v, got %v\n", expected-1, len(args.Args)-1)
					os.Exit(2)
				}
				if args.Verbose {
					logger.Println("Running target:", "BuildControlPlaneController")
				}
				
				wrapFn := func(ctx context.Context) error {
					return BuildControlPlaneController()
				}
				ret := runTarget(logger, wrapFn)
				handleError(logger, ret)
			case "buildcontrolplanecontrollerimage":
				expected := x + 0
				if expected > len(args.Args) {
					// note that expected and args at this point include the arg for the target itself
					// so we subtract 1 here to show the number of args without the target.
					logger.Printf("not enough arguments for target \"BuildControlPlaneControllerImage\", expected %v, got %v\n", expected-1, len(args.Args)-1)
					os.Exit(2)
				}
				if args.Verbose {
					logger.Println("Running target:", "BuildControlPlaneControllerImage")
				}
				
				wrapFn := func(ctx context.Context) error {
					return BuildControlPlaneControllerImage()
				}
				ret := runTarget(logger, wrapFn)
				handleError(logger, ret)
			case "builddatabasemigrator":
				expected := x + 0
				if expected > len(args.Args) {
					// note that expected and args at this point include the arg for the target itself
					// so we subtract 1 here to show the number of args without the target.
					logger.Printf("not enough arguments for target \"BuildDatabaseMigrator\", expected %v, got %v\n", expected-1, len(args.Args)-1)
					os.Exit(2)
				}
				if args.Verbose {
					logger.Println("Running target:", "BuildDatabaseMigrator")
				}
				
				wrapFn := func(ctx context.Context) error {
					return BuildDatabaseMigrator()
				}
				ret := runTarget(logger, wrapFn)
				handleError(logger, ret)
			case "builddatabasemigratorimage":
				expected := x + 0
				if expected > len(args.Args) {
					// note that expected and args at this point include the arg for the target itself
					// so we subtract 1 here to show the number of args without the target.
					logger.Printf("not enough arguments for target \"BuildDatabaseMigratorImage\", expected %v, got %v\n", expected-1, len(args.Args)-1)
					os.Exit(2)
				}
				if args.Verbose {
					logger.Println("Running target:", "BuildDatabaseMigratorImage")
				}
				
				wrapFn := func(ctx context.Context) error {
					return BuildDatabaseMigratorImage()
				}
				ret := runTarget(logger, wrapFn)
				handleError(logger, ret)
			case "buildgatewaycontroller":
				expected := x + 0
				if expected > len(args.Args) {
					// note that expected and args at this point include the arg for the target itself
					// so we subtract 1 here to show the number of args without the target.
					logger.Printf("not enough arguments for target \"BuildGatewayController\", expected %v, got %v\n", expected-1, len(args.Args)-1)
					os.Exit(2)
				}
				if args.Verbose {
					logger.Println("Running target:", "BuildGatewayController")
				}
				
				wrapFn := func(ctx context.Context) error {
					return BuildGatewayController()
				}
				ret := runTarget(logger, wrapFn)
				handleError(logger, ret)
			case "buildgatewaycontrollerimage":
				expected := x + 0
				if expected > len(args.Args) {
					// note that expected and args at this point include the arg for the target itself
					// so we subtract 1 here to show the number of args without the target.
					logger.Printf("not enough arguments for target \"BuildGatewayControllerImage\", expected %v, got %v\n", expected-1, len(args.Args)-1)
					os.Exit(2)
				}
				if args.Verbose {
					logger.Println("Running target:", "BuildGatewayControllerImage")
				}
				
				wrapFn := func(ctx context.Context) error {
					return BuildGatewayControllerImage()
				}
				ret := runTarget(logger, wrapFn)
				handleError(logger, ret)
			case "buildhelmworkloadcontroller":
				expected := x + 0
				if expected > len(args.Args) {
					// note that expected and args at this point include the arg for the target itself
					// so we subtract 1 here to show the number of args without the target.
					logger.Printf("not enough arguments for target \"BuildHelmWorkloadController\", expected %v, got %v\n", expected-1, len(args.Args)-1)
					os.Exit(2)
				}
				if args.Verbose {
					logger.Println("Running target:", "BuildHelmWorkloadController")
				}
				
				wrapFn := func(ctx context.Context) error {
					return BuildHelmWorkloadController()
				}
				ret := runTarget(logger, wrapFn)
				handleError(logger, ret)
			case "buildhelmworkloadcontrollerimage":
				expected := x + 0
				if expected > len(args.Args) {
					// note that expected and args at this point include the arg for the target itself
					// so we subtract 1 here to show the number of args without the target.
					logger.Printf("not enough arguments for target \"BuildHelmWorkloadControllerImage\", expected %v, got %v\n", expected-1, len(args.Args)-1)
					os.Exit(2)
				}
				if args.Verbose {
					logger.Println("Running target:", "BuildHelmWorkloadControllerImage")
				}
				
				wrapFn := func(ctx context.Context) error {
					return BuildHelmWorkloadControllerImage()
				}
				ret := runTarget(logger, wrapFn)
				handleError(logger, ret)
			case "buildimage":
				expected := x + 6
				if expected > len(args.Args) {
					// note that expected and args at this point include the arg for the target itself
					// so we subtract 1 here to show the number of args without the target.
					logger.Printf("not enough arguments for target \"BuildImage\", expected %v, got %v\n", expected-1, len(args.Args)-1)
					os.Exit(2)
				}
				if args.Verbose {
					logger.Println("Running target:", "BuildImage")
				}
				
			arg0 := args.Args[x]
			x++
			arg1 := args.Args[x]
			x++
			arg2 := args.Args[x]
			x++
				arg3, err := strconv.ParseBool(args.Args[x])
				if err != nil {
					logger.Printf("can't convert argument %q to bool\n", args.Args[x])
					os.Exit(2)
				}
				x++
				arg4, err := strconv.ParseBool(args.Args[x])
				if err != nil {
					logger.Printf("can't convert argument %q to bool\n", args.Args[x])
					os.Exit(2)
				}
				x++
			arg5 := args.Args[x]
			x++
				wrapFn := func(ctx context.Context) error {
					return BuildImage(arg0, arg1, arg2, arg3, arg4, arg5)
				}
				ret := runTarget(logger, wrapFn)
				handleError(logger, ret)
			case "buildkubernetesruntimecontroller":
				expected := x + 0
				if expected > len(args.Args) {
					// note that expected and args at this point include the arg for the target itself
					// so we subtract 1 here to show the number of args without the target.
					logger.Printf("not enough arguments for target \"BuildKubernetesRuntimeController\", expected %v, got %v\n", expected-1, len(args.Args)-1)
					os.Exit(2)
				}
				if args.Verbose {
					logger.Println("Running target:", "BuildKubernetesRuntimeController")
				}
				
				wrapFn := func(ctx context.Context) error {
					return BuildKubernetesRuntimeController()
				}
				ret := runTarget(logger, wrapFn)
				handleError(logger, ret)
			case "buildkubernetesruntimecontrollerimage":
				expected := x + 0
				if expected > len(args.Args) {
					// note that expected and args at this point include the arg for the target itself
					// so we subtract 1 here to show the number of args without the target.
					logger.Printf("not enough arguments for target \"BuildKubernetesRuntimeControllerImage\", expected %v, got %v\n", expected-1, len(args.Args)-1)
					os.Exit(2)
				}
				if args.Verbose {
					logger.Println("Running target:", "BuildKubernetesRuntimeControllerImage")
				}
				
				wrapFn := func(ctx context.Context) error {
					return BuildKubernetesRuntimeControllerImage()
				}
				ret := runTarget(logger, wrapFn)
				handleError(logger, ret)
			case "buildobservabilitycontroller":
				expected := x + 0
				if expected > len(args.Args) {
					// note that expected and args at this point include the arg for the target itself
					// so we subtract 1 here to show the number of args without the target.
					logger.Printf("not enough arguments for target \"BuildObservabilityController\", expected %v, got %v\n", expected-1, len(args.Args)-1)
					os.Exit(2)
				}
				if args.Verbose {
					logger.Println("Running target:", "BuildObservabilityController")
				}
				
				wrapFn := func(ctx context.Context) error {
					return BuildObservabilityController()
				}
				ret := runTarget(logger, wrapFn)
				handleError(logger, ret)
			case "buildobservabilitycontrollerimage":
				expected := x + 0
				if expected > len(args.Args) {
					// note that expected and args at this point include the arg for the target itself
					// so we subtract 1 here to show the number of args without the target.
					logger.Printf("not enough arguments for target \"BuildObservabilityControllerImage\", expected %v, got %v\n", expected-1, len(args.Args)-1)
					os.Exit(2)
				}
				if args.Verbose {
					logger.Println("Running target:", "BuildObservabilityControllerImage")
				}
				
				wrapFn := func(ctx context.Context) error {
					return BuildObservabilityControllerImage()
				}
				ret := runTarget(logger, wrapFn)
				handleError(logger, ret)
			case "buildsecretcontroller":
				expected := x + 0
				if expected > len(args.Args) {
					// note that expected and args at this point include the arg for the target itself
					// so we subtract 1 here to show the number of args without the target.
					logger.Printf("not enough arguments for target \"BuildSecretController\", expected %v, got %v\n", expected-1, len(args.Args)-1)
					os.Exit(2)
				}
				if args.Verbose {
					logger.Println("Running target:", "BuildSecretController")
				}
				
				wrapFn := func(ctx context.Context) error {
					return BuildSecretController()
				}
				ret := runTarget(logger, wrapFn)
				handleError(logger, ret)
			case "buildsecretcontrollerimage":
				expected := x + 0
				if expected > len(args.Args) {
					// note that expected and args at this point include the arg for the target itself
					// so we subtract 1 here to show the number of args without the target.
					logger.Printf("not enough arguments for target \"BuildSecretControllerImage\", expected %v, got %v\n", expected-1, len(args.Args)-1)
					os.Exit(2)
				}
				if args.Verbose {
					logger.Println("Running target:", "BuildSecretControllerImage")
				}
				
				wrapFn := func(ctx context.Context) error {
					return BuildSecretControllerImage()
				}
				ret := runTarget(logger, wrapFn)
				handleError(logger, ret)
			case "buildterraformcontroller":
				expected := x + 0
				if expected > len(args.Args) {
					// note that expected and args at this point include the arg for the target itself
					// so we subtract 1 here to show the number of args without the target.
					logger.Printf("not enough arguments for target \"BuildTerraformController\", expected %v, got %v\n", expected-1, len(args.Args)-1)
					os.Exit(2)
				}
				if args.Verbose {
					logger.Println("Running target:", "BuildTerraformController")
				}
				
				wrapFn := func(ctx context.Context) error {
					return BuildTerraformController()
				}
				ret := runTarget(logger, wrapFn)
				handleError(logger, ret)
			case "buildterraformcontrollerimage":
				expected := x + 0
				if expected > len(args.Args) {
					// note that expected and args at this point include the arg for the target itself
					// so we subtract 1 here to show the number of args without the target.
					logger.Printf("not enough arguments for target \"BuildTerraformControllerImage\", expected %v, got %v\n", expected-1, len(args.Args)-1)
					os.Exit(2)
				}
				if args.Verbose {
					logger.Println("Running target:", "BuildTerraformControllerImage")
				}
				
				wrapFn := func(ctx context.Context) error {
					return BuildTerraformControllerImage()
				}
				ret := runTarget(logger, wrapFn)
				handleError(logger, ret)
			case "buildtptctl":
				expected := x + 0
				if expected > len(args.Args) {
					// note that expected and args at this point include the arg for the target itself
					// so we subtract 1 here to show the number of args without the target.
					logger.Printf("not enough arguments for target \"BuildTptctl\", expected %v, got %v\n", expected-1, len(args.Args)-1)
					os.Exit(2)
				}
				if args.Verbose {
					logger.Println("Running target:", "BuildTptctl")
				}
				
				wrapFn := func(ctx context.Context) error {
					return BuildTptctl()
				}
				ret := runTarget(logger, wrapFn)
				handleError(logger, ret)
			case "buildtptdev":
				expected := x + 0
				if expected > len(args.Args) {
					// note that expected and args at this point include the arg for the target itself
					// so we subtract 1 here to show the number of args without the target.
					logger.Printf("not enough arguments for target \"BuildTptdev\", expected %v, got %v\n", expected-1, len(args.Args)-1)
					os.Exit(2)
				}
				if args.Verbose {
					logger.Println("Running target:", "BuildTptdev")
				}
				
				wrapFn := func(ctx context.Context) error {
					return BuildTptdev()
				}
				ret := runTarget(logger, wrapFn)
				handleError(logger, ret)
			case "buildworkloadcontroller":
				expected := x + 0
				if expected > len(args.Args) {
					// note that expected and args at this point include the arg for the target itself
					// so we subtract 1 here to show the number of args without the target.
					logger.Printf("not enough arguments for target \"BuildWorkloadController\", expected %v, got %v\n", expected-1, len(args.Args)-1)
					os.Exit(2)
				}
				if args.Verbose {
					logger.Println("Running target:", "BuildWorkloadController")
				}
				
				wrapFn := func(ctx context.Context) error {
					return BuildWorkloadController()
				}
				ret := runTarget(logger, wrapFn)
				handleError(logger, ret)
			case "buildworkloadcontrollerimage":
				expected := x + 0
				if expected > len(args.Args) {
					// note that expected and args at this point include the arg for the target itself
					// so we subtract 1 here to show the number of args without the target.
					logger.Printf("not enough arguments for target \"BuildWorkloadControllerImage\", expected %v, got %v\n", expected-1, len(args.Args)-1)
					os.Exit(2)
				}
				if args.Verbose {
					logger.Println("Running target:", "BuildWorkloadControllerImage")
				}
				
				wrapFn := func(ctx context.Context) error {
					return BuildWorkloadControllerImage()
				}
				ret := runTarget(logger, wrapFn)
				handleError(logger, ret)
			case "cleanlocalregistry":
				expected := x + 0
				if expected > len(args.Args) {
					// note that expected and args at this point include the arg for the target itself
					// so we subtract 1 here to show the number of args without the target.
					logger.Printf("not enough arguments for target \"CleanLocalRegistry\", expected %v, got %v\n", expected-1, len(args.Args)-1)
					os.Exit(2)
				}
				if args.Verbose {
					logger.Println("Running target:", "CleanLocalRegistry")
				}
				
				wrapFn := func(ctx context.Context) error {
					return CleanLocalRegistry()
				}
				ret := runTarget(logger, wrapFn)
				handleError(logger, ret)
			case "createlocalregistry":
				expected := x + 0
				if expected > len(args.Args) {
					// note that expected and args at this point include the arg for the target itself
					// so we subtract 1 here to show the number of args without the target.
					logger.Printf("not enough arguments for target \"CreateLocalRegistry\", expected %v, got %v\n", expected-1, len(args.Args)-1)
					os.Exit(2)
				}
				if args.Verbose {
					logger.Println("Running target:", "CreateLocalRegistry")
				}
				
				wrapFn := func(ctx context.Context) error {
					return CreateLocalRegistry()
				}
				ret := runTarget(logger, wrapFn)
				handleError(logger, ret)
			case "devdown":
				expected := x + 0
				if expected > len(args.Args) {
					// note that expected and args at this point include the arg for the target itself
					// so we subtract 1 here to show the number of args without the target.
					logger.Printf("not enough arguments for target \"DevDown\", expected %v, got %v\n", expected-1, len(args.Args)-1)
					os.Exit(2)
				}
				if args.Verbose {
					logger.Println("Running target:", "DevDown")
				}
				
				wrapFn := func(ctx context.Context) error {
					return DevDown()
				}
				ret := runTarget(logger, wrapFn)
				handleError(logger, ret)
			case "devforwardapi":
				expected := x + 0
				if expected > len(args.Args) {
					// note that expected and args at this point include the arg for the target itself
					// so we subtract 1 here to show the number of args without the target.
					logger.Printf("not enough arguments for target \"DevForwardAPI\", expected %v, got %v\n", expected-1, len(args.Args)-1)
					os.Exit(2)
				}
				if args.Verbose {
					logger.Println("Running target:", "DevForwardAPI")
				}
				
				wrapFn := func(ctx context.Context) error {
					return DevForwardAPI()
				}
				ret := runTarget(logger, wrapFn)
				handleError(logger, ret)
			case "devforwardcrdb":
				expected := x + 0
				if expected > len(args.Args) {
					// note that expected and args at this point include the arg for the target itself
					// so we subtract 1 here to show the number of args without the target.
					logger.Printf("not enough arguments for target \"DevForwardCrdb\", expected %v, got %v\n", expected-1, len(args.Args)-1)
					os.Exit(2)
				}
				if args.Verbose {
					logger.Println("Running target:", "DevForwardCrdb")
				}
				
				wrapFn := func(ctx context.Context) error {
					return DevForwardCrdb()
				}
				ret := runTarget(logger, wrapFn)
				handleError(logger, ret)
			case "devforwardnats":
				expected := x + 0
				if expected > len(args.Args) {
					// note that expected and args at this point include the arg for the target itself
					// so we subtract 1 here to show the number of args without the target.
					logger.Printf("not enough arguments for target \"DevForwardNats\", expected %v, got %v\n", expected-1, len(args.Args)-1)
					os.Exit(2)
				}
				if args.Verbose {
					logger.Println("Running target:", "DevForwardNats")
				}
				
				wrapFn := func(ctx context.Context) error {
					return DevForwardNats()
				}
				ret := runTarget(logger, wrapFn)
				handleError(logger, ret)
			case "devimage":
				expected := x + 6
				if expected > len(args.Args) {
					// note that expected and args at this point include the arg for the target itself
					// so we subtract 1 here to show the number of args without the target.
					logger.Printf("not enough arguments for target \"DevImage\", expected %v, got %v\n", expected-1, len(args.Args)-1)
					os.Exit(2)
				}
				if args.Verbose {
					logger.Println("Running target:", "DevImage")
				}
				
			arg0 := args.Args[x]
			x++
			arg1 := args.Args[x]
			x++
			arg2 := args.Args[x]
			x++
			arg3 := args.Args[x]
			x++
				arg4, err := strconv.ParseBool(args.Args[x])
				if err != nil {
					logger.Printf("can't convert argument %q to bool\n", args.Args[x])
					os.Exit(2)
				}
				x++
				arg5, err := strconv.ParseBool(args.Args[x])
				if err != nil {
					logger.Printf("can't convert argument %q to bool\n", args.Args[x])
					os.Exit(2)
				}
				x++
				wrapFn := func(ctx context.Context) error {
					return DevImage(arg0, arg1, arg2, arg3, arg4, arg5)
				}
				ret := runTarget(logger, wrapFn)
				handleError(logger, ret)
			case "devup":
				expected := x + 0
				if expected > len(args.Args) {
					// note that expected and args at this point include the arg for the target itself
					// so we subtract 1 here to show the number of args without the target.
					logger.Printf("not enough arguments for target \"DevUp\", expected %v, got %v\n", expected-1, len(args.Args)-1)
					os.Exit(2)
				}
				if args.Verbose {
					logger.Println("Running target:", "DevUp")
				}
				
				wrapFn := func(ctx context.Context) error {
					return DevUp()
				}
				ret := runTarget(logger, wrapFn)
				handleError(logger, ret)
			case "docs":
				expected := x + 0
				if expected > len(args.Args) {
					// note that expected and args at this point include the arg for the target itself
					// so we subtract 1 here to show the number of args without the target.
					logger.Printf("not enough arguments for target \"Docs\", expected %v, got %v\n", expected-1, len(args.Args)-1)
					os.Exit(2)
				}
				if args.Verbose {
					logger.Println("Running target:", "Docs")
				}
				
				wrapFn := func(ctx context.Context) error {
					return Docs()
				}
				ret := runTarget(logger, wrapFn)
				handleError(logger, ret)
			case "e2e":
				expected := x + 3
				if expected > len(args.Args) {
					// note that expected and args at this point include the arg for the target itself
					// so we subtract 1 here to show the number of args without the target.
					logger.Printf("not enough arguments for target \"E2e\", expected %v, got %v\n", expected-1, len(args.Args)-1)
					os.Exit(2)
				}
				if args.Verbose {
					logger.Println("Running target:", "E2e")
				}
				
			arg0 := args.Args[x]
			x++
			arg1 := args.Args[x]
			x++
				arg2, err := strconv.ParseBool(args.Args[x])
				if err != nil {
					logger.Printf("can't convert argument %q to bool\n", args.Args[x])
					os.Exit(2)
				}
				x++
				wrapFn := func(ctx context.Context) error {
					return E2e(arg0, arg1, arg2)
				}
				ret := runTarget(logger, wrapFn)
				handleError(logger, ret)
			case "e2eclean":
				expected := x + 0
				if expected > len(args.Args) {
					// note that expected and args at this point include the arg for the target itself
					// so we subtract 1 here to show the number of args without the target.
					logger.Printf("not enough arguments for target \"E2eClean\", expected %v, got %v\n", expected-1, len(args.Args)-1)
					os.Exit(2)
				}
				if args.Verbose {
					logger.Println("Running target:", "E2eClean")
				}
				
				wrapFn := func(ctx context.Context) error {
					return E2eClean()
				}
				ret := runTarget(logger, wrapFn)
				handleError(logger, ret)
			case "e2elocal":
				expected := x + 0
				if expected > len(args.Args) {
					// note that expected and args at this point include the arg for the target itself
					// so we subtract 1 here to show the number of args without the target.
					logger.Printf("not enough arguments for target \"E2eLocal\", expected %v, got %v\n", expected-1, len(args.Args)-1)
					os.Exit(2)
				}
				if args.Verbose {
					logger.Println("Running target:", "E2eLocal")
				}
				
				wrapFn := func(ctx context.Context) error {
					return E2eLocal()
				}
				ret := runTarget(logger, wrapFn)
				handleError(logger, ret)
			case "generate":
				expected := x + 0
				if expected > len(args.Args) {
					// note that expected and args at this point include the arg for the target itself
					// so we subtract 1 here to show the number of args without the target.
					logger.Printf("not enough arguments for target \"Generate\", expected %v, got %v\n", expected-1, len(args.Args)-1)
					os.Exit(2)
				}
				if args.Verbose {
					logger.Println("Running target:", "Generate")
				}
				
				wrapFn := func(ctx context.Context) error {
					return Generate()
				}
				ret := runTarget(logger, wrapFn)
				handleError(logger, ret)
			case "generatecode":
				expected := x + 0
				if expected > len(args.Args) {
					// note that expected and args at this point include the arg for the target itself
					// so we subtract 1 here to show the number of args without the target.
					logger.Printf("not enough arguments for target \"GenerateCode\", expected %v, got %v\n", expected-1, len(args.Args)-1)
					os.Exit(2)
				}
				if args.Verbose {
					logger.Println("Running target:", "GenerateCode")
				}
				
				wrapFn := func(ctx context.Context) error {
					return GenerateCode()
				}
				ret := runTarget(logger, wrapFn)
				handleError(logger, ret)
			case "generatedocs":
				expected := x + 0
				if expected > len(args.Args) {
					// note that expected and args at this point include the arg for the target itself
					// so we subtract 1 here to show the number of args without the target.
					logger.Printf("not enough arguments for target \"GenerateDocs\", expected %v, got %v\n", expected-1, len(args.Args)-1)
					os.Exit(2)
				}
				if args.Verbose {
					logger.Println("Running target:", "GenerateDocs")
				}
				
				wrapFn := func(ctx context.Context) error {
					return GenerateDocs()
				}
				ret := runTarget(logger, wrapFn)
				handleError(logger, ret)
			case "installsdk":
				expected := x + 0
				if expected > len(args.Args) {
					// note that expected and args at this point include the arg for the target itself
					// so we subtract 1 here to show the number of args without the target.
					logger.Printf("not enough arguments for target \"InstallSdk\", expected %v, got %v\n", expected-1, len(args.Args)-1)
					os.Exit(2)
				}
				if args.Verbose {
					logger.Println("Running target:", "InstallSdk")
				}
				
				wrapFn := func(ctx context.Context) error {
					return InstallSdk()
				}
				ret := runTarget(logger, wrapFn)
				handleError(logger, ret)
			case "installtptctl":
				expected := x + 0
				if expected > len(args.Args) {
					// note that expected and args at this point include the arg for the target itself
					// so we subtract 1 here to show the number of args without the target.
					logger.Printf("not enough arguments for target \"InstallTptctl\", expected %v, got %v\n", expected-1, len(args.Args)-1)
					os.Exit(2)
				}
				if args.Verbose {
					logger.Println("Running target:", "InstallTptctl")
				}
				
				wrapFn := func(ctx context.Context) error {
					return InstallTptctl()
				}
				ret := runTarget(logger, wrapFn)
				handleError(logger, ret)
			case "installtptdev":
				expected := x + 0
				if expected > len(args.Args) {
					// note that expected and args at this point include the arg for the target itself
					// so we subtract 1 here to show the number of args without the target.
					logger.Printf("not enough arguments for target \"InstallTptdev\", expected %v, got %v\n", expected-1, len(args.Args)-1)
					os.Exit(2)
				}
				if args.Verbose {
					logger.Println("Running target:", "InstallTptdev")
				}
				
				wrapFn := func(ctx context.Context) error {
					return InstallTptdev()
				}
				ret := runTarget(logger, wrapFn)
				handleError(logger, ret)
			case "integration":
				expected := x + 0
				if expected > len(args.Args) {
					// note that expected and args at this point include the arg for the target itself
					// so we subtract 1 here to show the number of args without the target.
					logger.Printf("not enough arguments for target \"Integration\", expected %v, got %v\n", expected-1, len(args.Args)-1)
					os.Exit(2)
				}
				if args.Verbose {
					logger.Println("Running target:", "Integration")
				}
				
				wrapFn := func(ctx context.Context) error {
					return Integration()
				}
				ret := runTarget(logger, wrapFn)
				handleError(logger, ret)
			case "testcommits":
				expected := x + 0
				if expected > len(args.Args) {
					// note that expected and args at this point include the arg for the target itself
					// so we subtract 1 here to show the number of args without the target.
					logger.Printf("not enough arguments for target \"TestCommits\", expected %v, got %v\n", expected-1, len(args.Args)-1)
					os.Exit(2)
				}
				if args.Verbose {
					logger.Println("Running target:", "TestCommits")
				}
				
				wrapFn := func(ctx context.Context) error {
					return TestCommits()
				}
				ret := runTarget(logger, wrapFn)
				handleError(logger, ret)
		
		default:
			logger.Printf("Unknown target specified: %q\n", target)
			os.Exit(2)
		}
	}
}




