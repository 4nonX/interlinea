package main

import (
	"fmt"
	"io"
	"os"
	"path/filepath"
	"strings"

	"github.com/4nonX/interlinea/filter/sdlpackage"
	"github.com/4nonX/interlinea/filter/xliff"
)

var packageCommands = []command{
	{"info", "show the languages and files of a Trados project package (.sdlppx)", runPackageInfo},
	{"extract", "write a package's bilingual files to a folder", runPackageExtract},
	{"return", "build a return package (.sdlrpx) from translated files", runPackageReturn},
}

func runPackage(args []string, stdin io.Reader, stdout, stderr io.Writer) error {
	if len(args) == 0 || args[0] == "-h" || args[0] == "--help" {
		fmt.Fprintln(stderr, "Usage: interlinea package <command> [options]")
		fmt.Fprintln(stderr)
		fmt.Fprintln(stderr, "Commands:")
		for _, c := range packageCommands {
			fmt.Fprintf(stderr, "  %-8s %s\n", c.name, c.summary)
		}
		if len(args) == 0 {
			return usageErr("missing package command")
		}
		return nil
	}
	for _, c := range packageCommands {
		if c.name == args[0] {
			return c.run(args[1:], stdin, stdout, stderr)
		}
	}
	return usageErr("unknown package command %q", args[0])
}

func openPackage(name string) ([]byte, *sdlpackage.Package, error) {
	data, err := os.ReadFile(name)
	if err != nil {
		return nil, nil, err
	}
	p, err := sdlpackage.Open(data)
	if err != nil {
		return nil, nil, fmt.Errorf("%s: %w", name, err)
	}
	return data, p, nil
}

func runPackageInfo(args []string, _ io.Reader, stdout, stderr io.Writer) error {
	fs := newFlags("package info", "package.sdlppx", stderr)
	asJSON := fs.Bool("json", false, "print as JSON")
	if err := parseFlags(fs, args); err != nil {
		return err
	}
	if fs.NArg() != 1 {
		return usageErr("expected one package")
	}
	data, p, err := openPackage(fs.Arg(0))
	if err != nil {
		return err
	}
	if *asJSON {
		return printJSON(stdout, p)
	}
	fmt.Fprintf(stdout, "Project:   %s (%s)\n", p.Name, p.Type)
	fmt.Fprintf(stdout, "Languages: %s → %s\n", p.SourceLang, strings.Join(p.TargetLangs, ", "))
	for _, f := range p.Files {
		role := "source"
		if f.Target {
			role = "to translate"
		}
		line := fmt.Sprintf("  %-50s %s", f.Path, role)
		if f.Target {
			if b, err := sdlpackage.Read(data, f.Path); err == nil {
				if ex, err := xliff.Extract(b); err == nil {
					done := 0
					for _, u := range ex.Units {
						if u.State.Rank() >= 2 {
							done++
						}
					}
					line += fmt.Sprintf(", %d of %d segments done", done, len(ex.Units))
				}
			}
		}
		fmt.Fprintln(stdout, line)
	}
	if p.Other > 0 {
		fmt.Fprintf(stdout, "  and %d other files (memories, termbases, settings)\n", p.Other)
	}
	return nil
}

func runPackageExtract(args []string, _ io.Reader, stdout, stderr io.Writer) error {
	fs := newFlags("package extract", "package.sdlppx", stderr)
	dir := fs.String("o", "", "folder to write the target files to (required)")
	all := fs.Bool("all", false, "also write the source-language files")
	if err := parseFlags(fs, args); err != nil {
		return err
	}
	if fs.NArg() != 1 {
		return usageErr("expected one package")
	}
	if *dir == "" {
		return usageErr("-o is required")
	}
	data, p, err := openPackage(fs.Arg(0))
	if err != nil {
		return err
	}
	n := 0
	for _, f := range p.Files {
		if !f.Target && !*all {
			continue
		}
		b, err := sdlpackage.Read(data, f.Path)
		if err != nil {
			return err
		}
		dst := filepath.Join(*dir, filepath.FromSlash(f.Path))
		if err := os.MkdirAll(filepath.Dir(dst), 0o755); err != nil {
			return err
		}
		if err := os.WriteFile(dst, b, 0o644); err != nil {
			return err
		}
		n++
	}
	fmt.Fprintf(stdout, "wrote %d files to %s\n", n, *dir)
	return nil
}

func runPackageReturn(args []string, _ io.Reader, stdout, stderr io.Writer) error {
	fs := newFlags("package return", "package.sdlppx folder", stderr)
	out := fs.String("o", "", "return package to write (required, .sdlrpx)")
	if err := parseFlags(fs, args); err != nil {
		return err
	}
	if fs.NArg() != 2 {
		return usageErr("expected the project package and the folder with the translated files (as 'package extract' wrote them)")
	}
	if *out == "" {
		return usageErr("-o is required")
	}
	data, p, err := openPackage(fs.Arg(0))
	if err != nil {
		return err
	}
	translated := map[string][]byte{}
	for _, f := range p.Files {
		if !f.Target {
			continue
		}
		b, err := os.ReadFile(filepath.Join(fs.Arg(1), filepath.FromSlash(f.Path)))
		if os.IsNotExist(err) {
			continue
		}
		if err != nil {
			return err
		}
		if _, err := xliff.Extract(b); err != nil {
			return fmt.Errorf("%s: %w", f.Path, err)
		}
		translated[f.Path] = b
	}
	if len(translated) == 0 {
		return fmt.Errorf("no translated files of the package in %s", fs.Arg(1))
	}
	rpx, err := sdlpackage.Return(data, translated)
	if err != nil {
		return err
	}
	if err := os.WriteFile(*out, rpx, 0o644); err != nil {
		return err
	}
	fmt.Fprintf(stdout, "wrote %s with %d translated files\n", *out, len(translated))
	return nil
}
