package publish_test

import (
	"context"
	"errors"
	"strings"
	"testing"

	"github.com/gsoultan/cronos/internal/app/publish"
	"github.com/gsoultan/cronos/internal/core/definition"
)

const letterhead = `
apiVersion: cronos.dev/v1
kind: Template
metadata: {name: letterhead}
spec:
  source: |
    #let header(ctx) = [*Acme Logistics* #h(1fr) #ctx.group]
`

// A report printed with a letterhead, its header naming the template.
func withHeader(name string) string {
	return report + `    - name: pdf
      renderer: paginated
      header: {template: ` + name + `}
      layout:
        - kind: table
          columns: [issued_at, status, total]
`
}

func TestAReportDrawsWithAPublishedTemplate(t *testing.T) {
	s, repo, _ := setup(t)
	s.WithReports(repo).WithTemplates(repo)
	mustPublish(t, s, dataset)
	mustPublish(t, s, letterhead)
	if _, err := repo.Template(context.Background(), "letterhead"); err != nil {
		t.Fatalf("the template is not live: %v", err)
	}
	mustPublish(t, s, withHeader("letterhead"))
}

func TestFurnitureIsCheckedWhenTheReportIsStored(t *testing.T) {
	cases := []struct {
		name, doc string
		want      error
		says      string
	}{
		{"a template nobody published", withHeader("nowhere"),
			publish.ErrNotFound, `template "nowhere"`},
		// A letterhead defines header(ctx). Used as a footer it would fail
		// every render of every recipient, at the hour the schedule fires.
		{"a header template used as a footer",
			strings.Replace(withHeader("letterhead"), "header: {template", "footer: {template", 1),
			definition.ErrInvalid, "defines no footer(ctx)"},
		{"both a template and a line of text",
			strings.Replace(withHeader("letterhead"), "{template: letterhead}",
				"{template: letterhead, text: Confidential}", 1),
			definition.ErrInvalid, "both a template and text"},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			s, repo, _ := setup(t)
			s.WithTemplates(repo)
			mustPublish(t, s, dataset)
			mustPublish(t, s, letterhead)
			_, err := s.Publish(context.Background(), []byte(c.doc), admin())
			if !errors.Is(err, c.want) || !strings.Contains(err.Error(), c.says) {
				t.Fatalf("got %v, want %v saying %q", err, c.want, c.says)
			}
		})
	}
}

// A template that imports a package would have the typesetter fetch it from
// the network inside the server.
func TestATemplateImportingAPackageIsRefused(t *testing.T) {
	s, _, _ := setup(t)
	doc := strings.Replace(letterhead, "#let header",
		"#import \"@preview/cetz:0.3.0\"\n    #let header", 1)
	_, err := s.Publish(context.Background(), []byte(doc), admin())
	if !errors.Is(err, definition.ErrInvalid) || !strings.Contains(err.Error(), "package") {
		t.Fatalf("got %v", err)
	}
}

func TestATemplateAReportDrawsWithIsNotDeleted(t *testing.T) {
	svc, store := service(catalog{reports: []definition.Report{{
		Name: "statement",
		Outputs: []definition.Output{{Name: "pdf",
			Footer: definition.Furniture{Template: "letterhead"}}},
	}}})
	err := svc.Delete(context.Background(), editor, "Template", "letterhead")
	if !errors.Is(err, publish.ErrInUse) || !strings.Contains(err.Error(), `report "statement"`) {
		t.Fatalf("got %v", err)
	}
	if store.deleted != "" {
		t.Fatal("it was deleted anyway")
	}
}
