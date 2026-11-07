package agent

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"os"
	"strings"

	"github.com/elentok/gx/apiclient"
	"github.com/elentok/gx/cmd/cmdbase"
	"github.com/elentok/gx/server/wire"
	"github.com/spf13/cobra"
)

// newDesignsDraftsCmd is `gx designs drafts`: the New Design drafts the web
// form keeps on the server. It calls the routes the form calls.
func newDesignsDraftsCmd(d Deps) *cobra.Command {
	cmd := &cobra.Command{Use: "drafts", Short: "New Design drafts kept by the server", Args: cobra.NoArgs}
	cmd.AddCommand(newDraftsListCmd(d), newDraftsShowCmd(d), newDraftsSaveCmd(d), newDraftsDeleteCmd(d), newDraftsLastProjectCmd(d), newDraftsImageCmd(d))
	return cmd
}

// newDraftsImageCmd is `gx designs drafts image`: the images of a draft.
func newDraftsImageCmd(d Deps) *cobra.Command {
	cmd := &cobra.Command{Use: "image", Short: "images of a draft", Args: cobra.NoArgs}
	cmd.AddCommand(newDraftsImageAddCmd(d), newDraftsImageGetCmd(d), newDraftsImageRmCmd(d))
	return cmd
}

func newDraftsImageGetCmd(d Deps) *cobra.Command {
	var jsonOut bool
	var out string
	cmd := &cobra.Command{
		Use:   "get <draft> <id>",
		Short: "print a draft image's bytes, or save them with -o",
		Args:  cobra.ExactArgs(2),
		RunE: func(c *cobra.Command, args []string) error {
			return runImageGet(c, d, jsonOut, out, args[1], func(ctx context.Context, cl *apiclient.Client) ([]byte, string, error) {
				return cl.Sessions.DesignDraftImage(ctx, args[0], args[1])
			})
		},
	}
	addImageGetFlags(cmd, &out, &jsonOut)
	return cmd
}

// newDesignsImagesCmd is `gx designs images`: the images/ folder of a map.
func newDesignsImagesCmd(d Deps) *cobra.Command {
	cmd := &cobra.Command{Use: "images", Short: "images/ folder of a map epic", Args: cobra.NoArgs}
	cmd.AddCommand(newDesignsImagesListCmd(d), newDesignsImagesGetCmd(d))
	return cmd
}

func newDesignsImagesListCmd(d Deps) *cobra.Command {
	var jsonOut bool
	cmd := &cobra.Command{
		Use:   "list <project:epic>",
		Short: "list a map epic's images in order",
		Args:  cobra.ExactArgs(1),
		RunE: func(c *cobra.Command, args []string) error {
			cl, err := d.Dial()()
			if err != nil {
				return err
			}
			items, err := cl.Sessions.DesignImages(c.Context(), args[0])
			if err != nil {
				return err
			}
			if jsonOut {
				return writeJSON(c.OutOrStdout(), items)
			}
			for _, it := range items {
				fmt.Fprintf(c.OutOrStdout(), "%s\t%s\n", it.ID, it.Name)
			}
			return nil
		},
	}
	cmd.Flags().BoolVar(&jsonOut, "json", false, "emit the list as JSON")
	return cmd
}

func newDesignsImagesGetCmd(d Deps) *cobra.Command {
	var jsonOut bool
	var out string
	cmd := &cobra.Command{
		Use:   "get <project:epic> <id>",
		Short: "print a map image's bytes, or save them with -o",
		Args:  cobra.ExactArgs(2),
		RunE: func(c *cobra.Command, args []string) error {
			return runImageGet(c, d, jsonOut, out, args[1], func(ctx context.Context, cl *apiclient.Client) ([]byte, string, error) {
				return cl.Sessions.DesignImage(ctx, args[0], args[1])
			})
		},
	}
	addImageGetFlags(cmd, &out, &jsonOut)
	return cmd
}

func newDraftsImageAddCmd(d Deps) *cobra.Command {
	var jsonOut bool
	cmd := &cobra.Command{
		Use:   "add <draft> <file>",
		Short: "attach an image file to a draft",
		Args:  cobra.ExactArgs(2),
		RunE: func(c *cobra.Command, args []string) error {
			cl, err := d.Dial()()
			if err != nil {
				return err
			}
			return runDraftsImageAdd(c.Context(), cl, c.OutOrStdout(), jsonOut, args[0], args[1])
		},
	}
	cmd.Flags().BoolVar(&jsonOut, "json", false, "emit the structured result (or refusal) as JSON")
	return cmd
}

func newDraftsImageRmCmd(d Deps) *cobra.Command {
	var jsonOut bool
	cmd := &cobra.Command{
		Use:   "rm <draft> <id>",
		Short: "remove an image from a draft",
		Args:  cobra.ExactArgs(2),
		RunE: func(c *cobra.Command, args []string) error {
			cl, err := d.Dial()()
			if err != nil {
				return err
			}
			return runDraftsImageRm(c.Context(), cl, c.OutOrStdout(), jsonOut, args[0], args[1])
		},
	}
	cmd.Flags().BoolVar(&jsonOut, "json", false, "emit the structured result (or refusal) as JSON")
	return cmd
}

// runDraftsImageAdd uploads the file's bytes; the server decides the type
// from the bytes, never from the file name.
func runDraftsImageAdd(ctx context.Context, cl *apiclient.Client, w io.Writer, jsonOut bool, draft, file string) error {
	data, err := os.ReadFile(file)
	if err != nil {
		return err
	}
	res, err := cl.Sessions.AddDesignDraftImage(ctx, draft, data)
	return printDraftImageResult(w, jsonOut, res, err)
}

func runDraftsImageRm(ctx context.Context, cl *apiclient.Client, w io.Writer, jsonOut bool, draft, image string) error {
	res, err := cl.Sessions.RemoveDesignDraftImage(ctx, draft, image)
	return printDraftImageResult(w, jsonOut, res, err)
}

func printDraftImageResult(w io.Writer, jsonOut bool, res wire.SessionImageResult, err error) error {
	if apiclient.IsNotRunning(err) {
		res, err = wire.SessionImageResult{Refused: true, Reason: wire.ReasonServerNotRunning, Message: "no server is running; start it with `gx server start`"}, nil
	}
	if err != nil {
		return fmt.Errorf("server write failed: %w", err)
	}
	return printImageResult(w, jsonOut, res)
}

// draftNotRunning is the refusal every drafts verb prints when no server
// answers: the verbs never start the server.
func draftNotRunning(err error) (wire.DesignDraftSaveResult, bool) {
	if apiclient.IsNotRunning(err) {
		return wire.DesignDraftSaveResult{Refused: true, Reason: wire.ReasonServerNotRunning}, true
	}
	return wire.DesignDraftSaveResult{}, false
}

func writeDraftJSON(w io.Writer, v any) error {
	stamped, err := cmdbase.StampProvenance(v, cmdbase.ViaServer, cmdbase.CallerActor(os.Getwd))
	if err != nil {
		return err
	}
	enc := json.NewEncoder(w)
	enc.SetIndent("", "  ")
	return enc.Encode(stamped)
}

// printDraftResult prints a read, save or delete result: the JSON form, or a
// refusal as an error, or text via ok.
func printDraftResult(w io.Writer, jsonOut bool, res wire.DesignDraftSaveResult, err error, ok func(wire.DesignDraftSaveResult) error) error {
	if r, down := draftNotRunning(err); down {
		res, err = r, nil
	}
	if err != nil {
		return fmt.Errorf("server call failed: %w", err)
	}
	if jsonOut {
		return writeDraftJSON(w, res)
	}
	if res.Refused {
		return fmt.Errorf("refused (%s)", res.Reason)
	}
	return ok(res)
}

func runDraftsList(ctx context.Context, cl *apiclient.Client, w io.Writer, jsonOut bool) error {
	items, err := cl.Sessions.DesignDrafts(ctx)
	if apiclient.IsNotRunning(err) {
		return fmt.Errorf("refused (%s): no server is running; start it with `gx server start`", wire.ReasonServerNotRunning)
	}
	if err != nil {
		return fmt.Errorf("server call failed: %w", err)
	}
	if jsonOut {
		return writeDraftJSON(w, struct {
			Drafts []wire.DesignDraftItem `json:"drafts"`
		}{items})
	}
	for _, it := range items {
		label := it.Name
		if label == "" {
			label = it.Slug
		}
		if label == "" {
			label, _, _ = strings.Cut(strings.TrimSpace(it.Idea), "\n")
		}
		if _, err := fmt.Fprintf(w, "%s\t%s\t%s\t%d images\n", it.ID, it.Project, label, it.ImageCount); err != nil {
			return err
		}
	}
	return nil
}

func runDraftsShow(ctx context.Context, cl *apiclient.Client, w io.Writer, jsonOut bool, id string) error {
	res, err := cl.Sessions.DesignDraft(ctx, id)
	return printDraftResult(w, jsonOut, res, err, func(res wire.DesignDraftSaveResult) error {
		_, err := fmt.Fprintln(w, res.Draft.Idea)
		return err
	})
}

func runDraftsDelete(ctx context.Context, cl *apiclient.Client, w io.Writer, jsonOut bool, id string) error {
	res, err := cl.Sessions.DeleteDesignDraft(ctx, id)
	return printDraftResult(w, jsonOut, res, err, func(wire.DesignDraftSaveResult) error { return nil })
}

func runDraftsSave(ctx context.Context, cl *apiclient.Client, w io.Writer, jsonOut bool, req wire.DesignDraftSaveRequest) error {
	res, err := cl.Sessions.SaveDesignDraft(ctx, req)
	return printDraftResult(w, jsonOut, res, err, func(res wire.DesignDraftSaveResult) error {
		if res.Deleted {
			return nil
		}
		_, err := fmt.Fprintf(w, "%s\trevision %d\n", res.Draft.ID, res.Draft.Revision)
		return err
	})
}

func newDraftsListCmd(d Deps) *cobra.Command {
	var jsonOut bool
	cmd := &cobra.Command{
		Use:   "list",
		Short: "list the drafts, most recently edited first",
		Args:  cobra.NoArgs,
		RunE: func(c *cobra.Command, _ []string) error {
			cl, err := d.Dial()()
			if err != nil {
				return err
			}
			return runDraftsList(c.Context(), cl, c.OutOrStdout(), jsonOut)
		},
	}
	cmd.Flags().BoolVar(&jsonOut, "json", false, "emit the list as JSON")
	return cmd
}

func newDraftsShowCmd(d Deps) *cobra.Command {
	var jsonOut bool
	cmd := &cobra.Command{
		Use:   "show <id>",
		Short: "print a draft's idea",
		Args:  cobra.ExactArgs(1),
		RunE: func(c *cobra.Command, args []string) error {
			cl, err := d.Dial()()
			if err != nil {
				return err
			}
			return runDraftsShow(c.Context(), cl, c.OutOrStdout(), jsonOut, args[0])
		},
	}
	cmd.Flags().BoolVar(&jsonOut, "json", false, "emit the whole draft (or refusal) as JSON")
	return cmd
}

func newDraftsDeleteCmd(d Deps) *cobra.Command {
	var jsonOut bool
	cmd := &cobra.Command{
		Use:   "delete <id>",
		Short: "delete a draft and its images",
		Args:  cobra.ExactArgs(1),
		RunE: func(c *cobra.Command, args []string) error {
			cl, err := d.Dial()()
			if err != nil {
				return err
			}
			return runDraftsDelete(c.Context(), cl, c.OutOrStdout(), jsonOut, args[0])
		},
	}
	cmd.Flags().BoolVar(&jsonOut, "json", false, "emit the structured result as JSON")
	return cmd
}

// runDraftsLastProject prints the shared last-used project, or sets it when
// project is non-empty and prints the new value.
func runDraftsLastProject(ctx context.Context, cl *apiclient.Client, w io.Writer, jsonOut bool, project string) error {
	var res wire.DesignLastProject
	var err error
	if project == "" {
		res, err = cl.Sessions.DesignLastProject(ctx)
	} else {
		res, err = cl.Sessions.SetDesignLastProject(ctx, project)
	}
	if apiclient.IsNotRunning(err) {
		return fmt.Errorf("refused (%s): no server is running; start it with `gx server start`", wire.ReasonServerNotRunning)
	}
	if err != nil {
		return fmt.Errorf("server call failed: %w", err)
	}
	if jsonOut {
		return writeDraftJSON(w, res)
	}
	_, err = fmt.Fprintln(w, res.Project)
	return err
}

func newDraftsLastProjectCmd(d Deps) *cobra.Command {
	var jsonOut bool
	cmd := &cobra.Command{
		Use:   "last-project [project]",
		Short: "print the last project the New Design form used, or set it",
		Args:  cobra.MaximumNArgs(1),
		RunE: func(c *cobra.Command, args []string) error {
			cl, err := d.Dial()()
			if err != nil {
				return err
			}
			project := ""
			if len(args) == 1 {
				project = args[0]
			}
			return runDraftsLastProject(c.Context(), cl, c.OutOrStdout(), jsonOut, project)
		},
	}
	cmd.Flags().BoolVar(&jsonOut, "json", false, "emit the result as JSON")
	return cmd
}

func newDraftsSaveCmd(d Deps) *cobra.Command {
	var jsonOut bool
	var req wire.DesignDraftSaveRequest
	cmd := &cobra.Command{
		Use:   "save <id> --revision <n> [--project p] [--slug s] [--idea text|-] [--name n]",
		Short: "save a draft's text fields against the revision you last saw",
		Args:  cobra.ExactArgs(1),
		RunE: func(c *cobra.Command, args []string) error {
			idea, err := readIdea(req.Idea, c.InOrStdin())
			if err != nil {
				return err
			}
			req.ID, req.Idea = args[0], idea
			cl, err := d.Dial()()
			if err != nil {
				return err
			}
			return runDraftsSave(c.Context(), cl, c.OutOrStdout(), jsonOut, req)
		},
	}
	cmd.Flags().IntVar(&req.Revision, "revision", 0, "the revision you last saw")
	cmd.Flags().StringVar(&req.Project, "project", "", "project")
	cmd.Flags().StringVar(&req.Slug, "slug", "", "slug")
	cmd.Flags().StringVar(&req.Idea, "idea", "", "idea, or - to read it from stdin")
	cmd.Flags().StringVar(&req.Name, "name", "", "name")
	cmd.Flags().BoolVar(&jsonOut, "json", false, "emit the structured result as JSON")
	return cmd
}
