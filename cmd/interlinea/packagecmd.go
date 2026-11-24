package main

import (
	"fmt"
	"io"
	"os"
	"path/filepath"
	"strings"

	"github.com/4nonX/interlinea/filter/formats"
	"github.com/4nonX/interlinea/filter/mqpackage"
	"github.com/4nonX/interlinea/filter/sdlpackage"
	"github.com/4nonX/interlinea/filter/xliff"
)

var packageCommands = []command{
	{"info", "show the languages and files of a Trados project package (.sdlppx) or a memoQ handoff package (.mqout)", runPackageInfo},
	{"extract", "write a package's bilingual files to a folder", runPackageExtract},
	{"return", "build a return package (.sdlrpx) or a memoQ delivery package (.mqback) from translated files", runPackageReturn},
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

// A package of either kind, as the commands use it.
type opened struct {
	*sdlpackage.Package
	data  []byte
	memoQ bool
}

// read returns a bilingual file of the package.
func (o *opened) read(path string) ([]byte, error) {
	if o.memoQ {
		return mqpackage.Read(o.data, path)
	}
	return sdlpackage.Read(o.data, path)
}

// onDisk is where a file of the package is written to and read from: a
// memoQ document kept in an archive is written as the memoQ XLIFF it is.
func (o *opened) onDisk(path string) string {
	if o.memoQ && strings.HasSuffix(strings.ToLower(path), ".mqxlz") {
		path = path[:len(path)-len(".mqxlz")] + ".mqxliff"
	}
	return filepath.FromSlash(path)
}

func (o *opened) returned(translated map[string][]byte) ([]byte, error) {
	if o.memoQ {
		return mqpackage.Return(o.data, translated)
	}
	return sdlpackage.Return(o.data, translated)
}

func openPackage(name string) ([]byte, *opened, error) {
	data, err := os.ReadFile(name)
	if err != nil {
		return nil, nil, err
	}
	if formats.Of(name) == formats.MemoQPackage {
		p, err := mqpackage.Open(data)
		if err != nil {
			return nil, nil, fmt.Errorf("%s: %w", name, err)
		}
		o := &opened{Package: &sdlpackage.Package{Name: p.Name, Type: "memoQ handoff package", SourceLang: p.SourceLang, TargetLangs: p.TargetLangs, Other: p.Other}, data: data, memoQ: true}
		for _, f := range p.Files {
			o.Files = append(o.Files, sdlpackage.File{Path: f.Path, Lang: f.Lang, Target: true, Size: f.Size})
		}
		return data, o, nil
	}
	p, err := sdlpackage.Open(data)
	if err != nil {
		return nil, nil, fmt.Errorf("%s: %w", name, err)
	}
	return data, &opened{Package: p, data: data}, nil
}

func runPackageInfo(args []string, _ io.Reader, stdout, stderr io.Writer) error {
	fs := newFlags("package info", "package", stderr)
	asJSON := fs.Bool("json", false, "print as JSON")
	if err := parseFlags(fs, args); err != nil {
		return err
	}
	if fs.NArg() != 1 {
		return usageErr("expected one package")
	}
	_, p, err := openPackage(fs.Arg(0))
	if err != nil {
		return err
	}
	if *asJSON {
		return printJSON(stdout, p.Package)
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
			if b, err := p.read(f.Path); err == nil {
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
	fs := newFlags("package extract", "package", stderr)
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
	_, p, err := openPackage(fs.Arg(0))
	if err != nil {
		return err
	}
	n := 0
	for _, f := range p.Files {
		if !f.Target && !*all {
			continue
		}
		b, err := p.read(f.Path)
		if err != nil {
			return err
		}
		dst := filepath.Join(*dir, p.onDisk(f.Path))
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
	fs := newFlags("package return", "package folder", stderr)
	out := fs.String("o", "", "return package to write (required; .sdlrpx for Trados, .mqback for memoQ)")
	if err := parseFlags(fs, args); err != nil {
		return err
	}
	if fs.NArg() != 2 {
		return usageErr("expected the project package and the folder with the translated files (as 'package extract' wrote them)")
	}
	if *out == "" {
		return usageErr("-o is required")
	}
	_, p, err := openPackage(fs.Arg(0))
	if err != nil {
		return err
	}
	translated := map[string][]byte{}
	for _, f := range p.Files {
		if !f.Target {
			continue
		}
		b, err := os.ReadFile(filepath.Join(fs.Arg(1), p.onDisk(f.Path)))
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
	rpx, err := p.returned(translated)
	if err != nil {
		return err
	}
	if err := os.WriteFile(*out, rpx, 0o644); err != nil {
		return err
	}
	fmt.Fprintf(stdout, "wrote %s with %d translated files\n", *out, len(translated))
	return nil
}
