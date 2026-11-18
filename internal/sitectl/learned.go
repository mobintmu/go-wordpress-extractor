package sitectl

import (
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"text/tabwriter"
	"time"

	"go-wordpress/internal/siteconfig"
)

const learnedUsage = `Usage: sitectl learned <action> [flags] [<domain>]

Actions:
  list               List learned configs with status and shadow matches
  show <domain>      Print one learned config
  activate <domain>  Make a candidate or demoted config serve its host now
  reject <domain>    Stop the config and never learn the host again
  adopt <domain>     Move the config into configs/sites/ as a normal,
                     git-tracked site config (drops the learned block);
                     finish it with a fixture and an e2e test

Flags: --dir configs  --learned-dir <dir> (default <dir>/learned, or APP_AI_CRAWLER_LEARN_DIR)  --json
`

// learned runs "sitectl learned <action>".
func (c CLI) learned(args []string) error {
	if len(args) == 0 || args[0] == "-h" || args[0] == "--help" || args[0] == "help" {
		fmt.Fprint(c.Stdout, learnedUsage)
		return nil
	}
	action := args[0]
	fs, com := c.flags("learned " + action)
	ldir := fs.String("learned-dir", os.Getenv("APP_AI_CRAWLER_LEARN_DIR"), "learned config directory")
	if err := fs.Parse(args[1:]); err != nil {
		return err
	}
	if *ldir == "" {
		*ldir = filepath.Join(*com.dir, "learned")
	}
	domain := ""
	if action != "list" {
		if fs.NArg() != 1 {
			return fmt.Errorf("sitectl learned %s needs one domain\n\n%s", action, learnedUsage)
		}
		domain = siteconfig.LearnedDomains(fs.Arg(0))[0]
	}
	path := siteconfig.LearnedFile(*ldir, domain)

	switch action {
	case "list":
		entries, err := siteconfig.LoadLearned(*ldir)
		if err != nil {
			return err
		}
		type row struct {
			File    string   `json:"file"`
			SiteID  string   `json:"site_id"`
			Status  string   `json:"status"`
			Matches []string `json:"shadow_matches"`
			Fails   int      `json:"failures_in_a_row"`
			Error   string   `json:"error,omitempty"`
		}
		rows := []row{}
		for _, e := range entries {
			r := row{File: filepath.Base(e.File), SiteID: e.SiteID, Status: e.LearnedStatus()}
			if e.Err != nil {
				r.Status, r.Error = "invalid", e.Err.Error()
			} else {
				r.Matches, r.Fails = e.Site.Learned.ShadowMatches, e.Site.Learned.FailuresInARow
			}
			rows = append(rows, r)
		}
		if *com.asJSON {
			return c.printJSON(rows)
		}
		tw := tabwriter.NewWriter(c.Stdout, 0, 4, 2, ' ', 0)
		fmt.Fprintln(tw, "FILE\tSITE_ID\tSTATUS\tSHADOW_MATCHES\tFAILURES")
		for _, r := range rows {
			fmt.Fprintf(tw, "%s\t%s\t%s\t%d\t%d\n", r.File, r.SiteID, r.Status, len(r.Matches), r.Fails)
		}
		return tw.Flush()

	case "show":
		b, err := os.ReadFile(path)
		if err != nil {
			return err
		}
		_, err = c.Stdout.Write(b)
		return err

	case "activate", "reject":
		raw, err := siteconfig.ReadLearnedRaw(path)
		if err != nil {
			return err
		}
		block, _ := raw["learned"].(map[string]any)
		if block == nil {
			return errors.New(filepath.Base(path) + " has no learned block")
		}
		status := siteconfig.LearnedActive
		if action == "reject" {
			status = siteconfig.LearnedRejected
		}
		block["status"] = status
		block["failures_in_a_row"] = 0
		raw["learned"] = block
		if action == "activate" {
			if _, err := siteconfig.ParseSite(raw); err != nil {
				return fmt.Errorf("%s is invalid, not activating: %w", filepath.Base(path), err)
			}
		}
		if err := siteconfig.WriteLearned(path, raw); err != nil {
			return err
		}
		fmt.Fprintf(c.Stdout, "%s: status %s\n", filepath.Base(path), status)
		return nil

	case "adopt":
		raw, err := siteconfig.ReadLearnedRaw(path)
		if err != nil {
			return err
		}
		dst := filepath.Join(*com.dir, siteconfig.SitesDir, filepath.Base(path))
		if _, err := os.Stat(dst); err == nil {
			return fmt.Errorf("%s already exists; merge by hand", dst)
		}
		block, _ := raw["learned"].(map[string]any)
		delete(raw, "learned")
		raw["site_id"] = nonID(domain)
		raw["version"] = time.Now().UTC().Format("2006-01-02") + ".1"
		raw["$comment"] = fmt.Sprintf("Adopted from selectors the AI crawler learned (%v, from %v). Check every selector against a saved page, add a fixture (sitectl fixture) and an e2e test.",
			block["model"], block["learned_from"])
		if _, err := siteconfig.ParseSite(raw); err != nil {
			return fmt.Errorf("the adopted config is invalid: %w", err)
		}
		if err := siteconfig.WriteLearned(dst, raw); err != nil {
			return err
		}
		if err := os.Remove(path); err != nil {
			return err
		}
		fmt.Fprintf(c.Stdout, "adopted %s as %s\n", filepath.Base(path), dst)
		return nil
	}
	return fmt.Errorf("unknown action %q\n\n%s", action, learnedUsage)
}

// nonID turns a domain into a site_id (dots become "-").
func nonID(domain string) string {
	out := []rune{}
	for _, r := range domain {
		switch {
		case r >= 'a' && r <= 'z', r >= '0' && r <= '9', r == '-', r == '_':
			out = append(out, r)
		default:
			out = append(out, '-')
		}
	}
	if len(out) > 64 {
		out = out[:64]
	}
	return string(out)
}
