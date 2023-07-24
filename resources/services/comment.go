package resources

import (
	"context"
	"fmt"
	"net/url"
	"time"

	"github.com/apache/arrow/go/v13/arrow"
	"github.com/cloudquery/plugin-sdk/v4/schema"
	"github.com/cloudquery/plugin-sdk/v4/transformers"
	"github.com/poozlehq/cq-ticketing/client"
	"github.com/poozlehq/cq-ticketing/internal/ticketing"
)

func Comment() *schema.Table {
	return &schema.Table{
		Name:      "ticketing_comment",
		Resolver:  fetchComment,
		Transform: transformers.TransformWithStruct(&ticketing.Comment{}),
		Columns: []schema.Column{
			{
				Name:       "id",
				Type:       arrow.BinaryTypes.String,
				Resolver:   schema.PathResolver("Id"),
				PrimaryKey: true,
			},
			{
				Name:       "integration_account_id",
				Type:       arrow.BinaryTypes.String,
				Resolver:   schema.PathResolver("IntegrationAccountId"),
				PrimaryKey: true,
			},
			{
				Name:           "updated_at",
				Type:           arrow.FixedWidthTypes.Timestamp_us,
				Resolver:       schema.PathResolver("UpdatedAt"),
				IncrementalKey: true,
			},
		},
		IsIncremental: true,
	}
}

func fetchComment(ctx context.Context, meta schema.ClientMeta, parent *schema.Resource, res chan<- interface{}) error {
	cl := meta.(*client.Client)

	ticket, ok := parent.Item.(ticketing.Ticket)
	if !ok {
		return fmt.Errorf("parent.Item is not of type *ticketing.Collection, it is of type %T", parent.Item)
	}
	key := fmt.Sprintf("ticketing-comment-%s-%s-%s", cl.Spec.WorkspaceId, cl.Spec.IntegrationAccountId, *ticket.Id)
	p := url.Values{}

	min, _ := time.Parse(time.RFC3339, cl.Spec.StartDate)
	if cl.Backend != nil {
		value, err := cl.Backend.GetKey(ctx, key)

		if err != nil {
			return fmt.Errorf("failed to retrieve state from backend: %w", err)
		}
		if value != "" {
			min, err = time.Parse(time.RFC3339, value)
			if err != nil {
				return fmt.Errorf("retrieved invalid state value: %q %w", value, err)
			}
		}
	}
	p.Set("since", min.Format(time.RFC3339))
	p.Set("raw", "true")
	p.Set("limit", "5")

	cursor := fmt.Sprintf("/%s/tickets/%s/comments", *ticket.CollectionId, *ticket.Id)
	for {
		ret, p, err := cl.Services.GetComment(ctx, cursor, p)
		if err != nil {
			return err
		}
		now := time.Now()
		for i := range ret.Data {
			ret.Data[i].CqCreatedAt = &now
			ret.Data[i].CqUpdatedAt = &now
			ret.Data[i].IntegrationAccountId = &cl.Spec.IntegrationAccountId
		}
		res <- ret.Data

		if p == nil {
			break
		}
	}

	if err := cl.Backend.SetKey(ctx, key, time.Now().Format(time.RFC3339)); err != nil {
		return fmt.Errorf("failed to store state to backend: %w", err)
	}

	return cl.Backend.Flush(ctx)
}