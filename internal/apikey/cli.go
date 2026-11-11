package apikey

import (
	"bufio"
	"encoding/csv"
	"encoding/json"
	"errors"
	"flag"
	"fmt"
	"io"
	"os"
	"strings"
	"text/tabwriter"
	"time"

	"go-wordpress/internal/config"
)

// CLI runs the apikey command. It is kept in this package so it can be
// tested without building a binary.
type CLI struct {
	Stdin  io.Reader
	Stdout io.Writer
	Stderr io.Writer
	Getenv func(string) string
	Now    func() time.Time
}

const cliUsage = `Usage: apikey <command> [flags]

Commands:
  generate-pepper     Print a new random server secret (APP_API_KEY_PEPPER)
  generate            Create one key
  generate-bulk       Create many keys and write them to a CSV file
  list                Show key metadata (never hashes)
  revoke              Revoke keys by --id or --name-prefix
  hash-ops-password   Read a password on stdin and print its keyed hash

All commands use the file in APP_API_KEYS_FILE unless --output is given.
`

// Run executes args and returns the process exit code.
func (c CLI) Run(args []string) int {
	if c.Getenv == nil {
		c.Getenv = os.Getenv
	}
	if c.Now == nil {
		c.Now = time.Now
	}
	if len(args) == 0 {
		fmt.Fprint(c.Stderr, cliUsage)
		return 2
	}
	var err error
	switch args[0] {
	case "generate-pepper":
		err = c.generatePepper()
	case "generate":
		err = c.generate(args[1:])
	case "generate-bulk":
		err = c.generateBulk(args[1:])
	case "list":
		err = c.list(args[1:])
	case "revoke":
		err = c.revoke(args[1:])
	case "hash-ops-password":
		err = c.hashOpsPassword()
	case "-h", "--help", "help":
		fmt.Fprint(c.Stdout, cliUsage)
		return 0
	default:
		fmt.Fprintf(c.Stderr, "unknown command %q\n\n%s", args[0], cliUsage)
		return 2
	}
	if err != nil {
		if errors.Is(err, flag.ErrHelp) {
			return 0
		}
		fmt.Fprintf(c.Stderr, "error: %v\n", err)
		return 1
	}
	return 0
}

func (c CLI) keyFile(output string) string {
	if output != "" {
		return output
	}
	if f := c.Getenv("APP_API_KEYS_FILE"); f != "" {
		return f
	}
	return config.DefaultAPIKeysFile(c.Getenv("APP_ENV"))
}

func (c CLI) pepper() (string, error) {
	p := c.Getenv("APP_API_KEY_PEPPER")
	if len(p) < config.MinPepperLength {
		return "", fmt.Errorf("APP_API_KEY_PEPPER is not set or shorter than %d characters; it must match the production pepper", config.MinPepperLength)
	}
	fmt.Fprintf(c.Stderr, "pepper fingerprint: %s (must match the server's startup log)\n", PepperFingerprint(p))
	return p, nil
}

func (c CLI) flags(name string) *flag.FlagSet {
	fs := flag.NewFlagSet(name, flag.ContinueOnError)
	fs.SetOutput(c.Stderr)
	return fs
}

func parseExpiry(s string) (time.Duration, error) {
	if s == "" || s == "0" || s == neverExpires {
		return 0, nil
	}
	d, err := time.ParseDuration(s)
	if err != nil || d < 0 {
		return 0, fmt.Errorf("invalid --expires-in %q: use a duration such as 8760h, or 'never'", s)
	}
	return d, nil
}

func (c CLI) generatePepper() error {
	p, err := NewPepper()
	if err != nil {
		return err
	}
	fmt.Fprintln(c.Stdout, p)
	return nil
}

func (c CLI) generate(args []string) error {
	fs := c.flags("generate")
	name := fs.String("name", "", "client application name (letters, digits, '-', '_')")
	expires := fs.String("expires-in", "8760h", "lifetime such as 8760h, or 'never'")
	output := fs.String("output", "", "key file (default APP_API_KEYS_FILE)")
	plain := fs.String("plain-output", "", "write the full key to this CSV instead of stdout")
	asJSON := fs.Bool("json", false, "print JSON")
	if err := fs.Parse(args); err != nil {
		return err
	}
	if *name == "" {
		return errors.New("--name is required")
	}
	exp, err := parseExpiry(*expires)
	if err != nil {
		return err
	}
	pepper, err := c.pepper()
	if err != nil {
		return err
	}
	g, err := Generate(pepper, *name, exp, c.Now())
	if err != nil {
		return err
	}
	path := c.keyFile(*output)
	if err := c.store(path, *plain, []Generated{g}); err != nil {
		return err
	}
	return c.report(path, *plain, []Generated{g}, *asJSON)
}

func (c CLI) generateBulk(args []string) error {
	fs := c.flags("generate-bulk")
	count := fs.Int("count", 0, "number of keys to create")
	prefix := fs.String("name-prefix", "", "names become <prefix>-001, <prefix>-002, ...")
	namesFile := fs.String("names-file", "", "file with one name per line")
	expires := fs.String("expires-in", "8760h", "lifetime such as 8760h, or 'never'")
	output := fs.String("output", "", "key file (default APP_API_KEYS_FILE)")
	plain := fs.String("plain-output", "", "CSV file that receives the full keys (required)")
	asJSON := fs.Bool("json", false, "print JSON")
	if err := fs.Parse(args); err != nil {
		return err
	}
	if *plain == "" {
		return errors.New("--plain-output is required for generate-bulk")
	}
	names, err := c.bulkNames(*count, *prefix, *namesFile)
	if err != nil {
		return err
	}
	exp, err := parseExpiry(*expires)
	if err != nil {
		return err
	}
	pepper, err := c.pepper()
	if err != nil {
		return err
	}
	now := c.Now()
	gens := make([]Generated, 0, len(names))
	ids := map[string]bool{}
	for _, n := range names {
		g, err := Generate(pepper, n, exp, now)
		if err != nil {
			return err
		}
		if ids[g.Entry.ID] {
			return fmt.Errorf("generated duplicate id %s; run again", g.Entry.ID)
		}
		ids[g.Entry.ID] = true
		gens = append(gens, g)
	}
	path := c.keyFile(*output)
	if err := c.store(path, *plain, gens); err != nil {
		return err
	}
	return c.report(path, *plain, gens, *asJSON)
}

func (c CLI) bulkNames(count int, prefix, namesFile string) ([]string, error) {
	var names []string
	switch {
	case namesFile != "" && (count > 0 || prefix != ""):
		return nil, errors.New("use either --names-file or --count with --name-prefix")
	case namesFile != "":
		f, err := os.Open(namesFile)
		if err != nil {
			return nil, err
		}
		defer f.Close()
		sc := bufio.NewScanner(f)
		for sc.Scan() {
			n := strings.TrimSpace(sc.Text())
			if n == "" || strings.HasPrefix(n, "#") {
				continue
			}
			names = append(names, n)
		}
		if err := sc.Err(); err != nil {
			return nil, err
		}
	case count > 0 && prefix != "":
		width := len(fmt.Sprint(count))
		if width < 3 {
			width = 3
		}
		for i := 1; i <= count; i++ {
			names = append(names, fmt.Sprintf("%s-%0*d", prefix, width, i))
		}
	default:
		return nil, errors.New("give --count and --name-prefix, or --names-file")
	}
	if len(names) == 0 {
		return nil, errors.New("no names given")
	}
	seen := map[string]bool{}
	for _, n := range names {
		if !ValidName(n) {
			return nil, fmt.Errorf("name %q may only contain letters, digits, '-' and '_'", n)
		}
		if seen[n] {
			return nil, fmt.Errorf("name %q appears twice", n)
		}
		seen[n] = true
	}
	return names, nil
}

// store writes the CSV first (when requested), then adds the entries to
// the key file. If the key file update fails, the CSV is removed, so no
// keys are created.
func (c CLI) store(path, plain string, gens []Generated) error {
	if existing, _, err := ReadFile(path, true); err != nil {
		return err
	} else if err := conflicts(existing, gens); err != nil {
		return err
	}
	if plain != "" {
		if err := writeCSV(plain, gens); err != nil {
			return err
		}
	}
	err := Update(path, func(entries []Entry) ([]Entry, error) {
		if err := conflicts(entries, gens); err != nil {
			return nil, err
		}
		for _, g := range gens {
			entries = append(entries, g.Entry)
		}
		return entries, nil
	})
	if err != nil {
		if plain != "" {
			os.Remove(plain) //nolint:errcheck
		}
		return fmt.Errorf("update key file %s: %w (no keys were created)", path, err)
	}
	return nil
}

func conflicts(existing []Entry, gens []Generated) error {
	ids, names := map[string]bool{}, map[string]bool{}
	for _, e := range existing {
		ids[e.ID] = true
		names[e.Name] = true
	}
	for _, g := range gens {
		if ids[g.Entry.ID] {
			return fmt.Errorf("key id %s already exists", g.Entry.ID)
		}
		if names[g.Entry.Name] {
			return fmt.Errorf("a key named %q already exists", g.Entry.Name)
		}
	}
	return nil
}

func writeCSV(path string, gens []Generated) error {
	f, err := os.OpenFile(path, os.O_WRONLY|os.O_CREATE|os.O_EXCL, 0o600)
	if err != nil {
		if errors.Is(err, os.ErrExist) {
			return fmt.Errorf("%s already exists; refusing to overwrite", path)
		}
		return err
	}
	w := csv.NewWriter(f)
	w.Write([]string{"id", "name", "key", "expires_at"}) //nolint:errcheck
	for _, g := range gens {
		w.Write([]string{g.Entry.ID, g.Entry.Name, g.Key, expiryText(g.Entry)}) //nolint:errcheck
	}
	w.Flush()
	if err := w.Error(); err != nil {
		f.Close()
		os.Remove(path) //nolint:errcheck
		return err
	}
	if err := f.Sync(); err != nil {
		f.Close()
		os.Remove(path) //nolint:errcheck
		return err
	}
	return f.Close()
}

func expiryText(e Entry) string {
	if e.ExpiresAt == nil {
		return neverExpires
	}
	return e.ExpiresAt.UTC().Format(time.RFC3339)
}

type generatedJSON struct {
	ID        string `json:"id"`
	Name      string `json:"name"`
	Key       string `json:"key,omitempty"`
	ExpiresAt string `json:"expires_at"`
}

// report prints the result. Full keys are printed only when no
// --plain-output file was given.
func (c CLI) report(path, plain string, gens []Generated, asJSON bool) error {
	showKey := plain == ""
	if asJSON {
		out := struct {
			KeyFile     string          `json:"key_file"`
			PlainOutput string          `json:"plain_output,omitempty"`
			Keys        []generatedJSON `json:"keys"`
		}{KeyFile: path, PlainOutput: plain}
		for _, g := range gens {
			j := generatedJSON{ID: g.Entry.ID, Name: g.Entry.Name, ExpiresAt: expiryText(g.Entry)}
			if showKey {
				j.Key = g.Key
			}
			out.Keys = append(out.Keys, j)
		}
		return json.NewEncoder(c.Stdout).Encode(out)
	}
	for _, g := range gens {
		fmt.Fprintf(c.Stdout, "created key %s (%s), expires %s\n", g.Entry.ID, g.Entry.Name, expiryText(g.Entry))
		if showKey {
			fmt.Fprintf(c.Stdout, "key (shown once, cannot be recovered): %s\n", g.Key)
		}
	}
	fmt.Fprintf(c.Stdout, "key file: %s\n", path)
	if plain != "" {
		fmt.Fprintf(c.Stdout, "full keys written to %s (permission 0600). Deliver it securely, then delete it.\n", plain)
	}
	return nil
}

func (c CLI) list(args []string) error {
	fs := c.flags("list")
	output := fs.String("output", "", "key file (default APP_API_KEYS_FILE)")
	asJSON := fs.Bool("json", false, "print JSON")
	if err := fs.Parse(args); err != nil {
		return err
	}
	entries, _, err := ReadFile(c.keyFile(*output), false)
	if err != nil {
		return err
	}
	type row struct {
		ID        string `json:"id"`
		Name      string `json:"name"`
		Status    string `json:"status"`
		CreatedAt string `json:"created_at"`
		ExpiresAt string `json:"expires_at"`
		Usable    bool   `json:"usable"`
	}
	rows := make([]row, 0, len(entries))
	now := c.Now()
	for _, e := range entries {
		rows = append(rows, row{e.ID, e.Name, e.Status, e.CreatedAt.UTC().Format(time.RFC3339), expiryText(e), e.Usable(now)})
	}
	if *asJSON {
		return json.NewEncoder(c.Stdout).Encode(rows)
	}
	tw := tabwriter.NewWriter(c.Stdout, 0, 0, 2, ' ', 0)
	fmt.Fprintln(tw, "ID\tNAME\tSTATUS\tCREATED\tEXPIRES\tUSABLE")
	for _, r := range rows {
		fmt.Fprintf(tw, "%s\t%s\t%s\t%s\t%s\t%t\n", r.ID, r.Name, r.Status, r.CreatedAt, r.ExpiresAt, r.Usable)
	}
	return tw.Flush()
}

func (c CLI) revoke(args []string) error {
	fs := c.flags("revoke")
	id := fs.String("id", "", "key id to revoke")
	prefix := fs.String("name-prefix", "", "revoke every key whose name starts with this prefix")
	output := fs.String("output", "", "key file (default APP_API_KEYS_FILE)")
	if err := fs.Parse(args); err != nil {
		return err
	}
	if (*id == "") == (*prefix == "") {
		return errors.New("give exactly one of --id or --name-prefix")
	}
	var revoked []string
	err := Update(c.keyFile(*output), func(entries []Entry) ([]Entry, error) {
		for i, e := range entries {
			if (*id != "" && e.ID == *id) || (*prefix != "" && strings.HasPrefix(e.Name, *prefix)) {
				if e.Status != StatusRevoked {
					entries[i].Status = StatusRevoked
					revoked = append(revoked, e.ID)
				}
			}
		}
		if len(revoked) == 0 {
			return nil, errors.New("no active key matched")
		}
		return entries, nil
	})
	if err != nil {
		return err
	}
	fmt.Fprintf(c.Stdout, "revoked %d key(s): %s\n", len(revoked), strings.Join(revoked, ", "))
	fmt.Fprintln(c.Stdout, "copy the key file to every instance, or edit each instance's file the same way")
	return nil
}

func (c CLI) hashOpsPassword() error {
	pepper, err := c.pepper()
	if err != nil {
		return err
	}
	line, err := bufio.NewReader(c.Stdin).ReadString('\n')
	if err != nil && !errors.Is(err, io.EOF) {
		return err
	}
	pw := strings.TrimRight(line, "\r\n")
	if len(pw) < 12 {
		return errors.New("password must be at least 12 characters")
	}
	fmt.Fprintln(c.Stdout, Hash(pepper, pw))
	return nil
}
