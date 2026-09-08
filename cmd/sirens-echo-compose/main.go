package main

import (
	"flag"
	"fmt"
	"log"
	"os"
	"path/filepath"
	"strings"

	"forgejo.coilysiren.me/coilyco-gaming/sirens-echo/internal/community"
)

// Expands the tracked role graph, stages the admitted bodies, and writes the
// declaration agent-compose consumes. See docs/sirens-echo-compose.md.
const sourceID = "aos-public"

func main() {
	var catalogs catalogList
	flag.Var(&catalogs, "catalog", "checkout supplying a composed catalogue; repeatable")
	role := flag.String("role", "", "role whose bindings to expand")
	compose := flag.String("compose-dir", "agent/compose", "directory holding roles.kdl")
	roster := flag.String("check-roster", "", "comma-separated roster; verify the graph names only these roles, then exit")
	flag.Parse()

	raw, err := os.ReadFile(filepath.Join(*compose, "roles.kdl"))
	if err != nil {
		log.Fatalf("read role graph: %v", err)
	}
	graph := community.ParseRoleGraph(string(raw))
	if err := community.CheckGraphGlobals(graph); err != nil {
		log.Fatal(err)
	}
	if *roster != "" {
		if err := community.CheckGraphRoles(graph, strings.Split(*roster, ",")); err != nil {
			log.Fatal(err)
		}
		fmt.Printf("role graph names %d roles, all present in the roster\n", len(graph.Patterns))
		return
	}
	if len(catalogs) == 0 || *role == "" {
		log.Fatal("--catalog and --role are required")
	}

	admitted, excluded, err := community.ExpandRoleWithExclusions(catalogs, *role, graph)
	if err != nil {
		log.Fatal(err)
	}
	names := community.SortedNames(admitted)

	staged := filepath.Join(*compose, "skills")
	if err := os.RemoveAll(staged); err != nil {
		log.Fatalf("clear staged tree: %v", err)
	}
	for _, name := range names {
		source := filepath.Join(admitted[name], ".agents", "composed", name)
		if err := stage(source, filepath.Join(staged, name)); err != nil {
			log.Fatalf("stage %s: %v", name, err)
		}
	}
	declaration := filepath.Join(*compose, sourceID+".kdl")
	if err := os.WriteFile(declaration, []byte(community.RenderDeclaration(sourceID, names)), 0o644); err != nil {
		log.Fatalf("write declaration: %v", err)
	}
	fmt.Printf("role %s: %d sources admitted\n", *role, len(names))
	for _, name := range names {
		fmt.Printf("  %s\t%s\n", name, admitted[name])
	}
	for _, drop := range excluded {
		fmt.Printf("  denied: %s\n", drop)
	}
}

// catalogList collects a repeatable --catalog, so a layer holding more than one
// checkout expands the same graph across all of them.
type catalogList []string

func (c *catalogList) String() string { return strings.Join(*c, ",") }

func (c *catalogList) Set(value string) error {
	*c = append(*c, value)
	return nil
}

// stage copies one composed body under its declared path. agent-compose expects
// SKILL.md at a declared skill path, so COMPOSED.md is renamed on the way in.
func stage(source, target string) error {
	if err := os.MkdirAll(target, 0o755); err != nil {
		return err
	}
	body, err := os.ReadFile(filepath.Join(source, "COMPOSED.md"))
	if err != nil {
		return err
	}
	if err := os.WriteFile(filepath.Join(target, "SKILL.md"), body, 0o644); err != nil {
		return err
	}
	references, err := os.ReadDir(filepath.Join(source, "references"))
	if err != nil {
		return nil
	}
	if err := os.MkdirAll(filepath.Join(target, "references"), 0o755); err != nil {
		return err
	}
	for _, entry := range references {
		if entry.IsDir() {
			continue
		}
		body, err := os.ReadFile(filepath.Join(source, "references", entry.Name()))
		if err != nil {
			return err
		}
		if err := os.WriteFile(filepath.Join(target, "references", entry.Name()), body, 0o644); err != nil {
			return err
		}
	}
	return nil
}
