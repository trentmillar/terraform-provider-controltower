package ops

import (
	"context"
	"fmt"
	"github.com/aws/aws-sdk-go/aws"
	"github.com/aws/aws-sdk-go/service/organizations"
	"github.com/hashicorp/terraform-plugin-sdk/v2/helper/retry"
)

func MoveAccount(ctx context.Context, conn *organizations.Organizations, accountId, sourceOu, destinationOu string) error {

	account, err := FindAccountByID(ctx, conn, accountId)
	if err != nil {
		return err
	}

	input := &organizations.MoveAccountInput{
		AccountId:           account.Id,
		DestinationParentId: aws.String(destinationOu),
		SourceParentId:      aws.String(sourceOu),
	}
	_, err = conn.MoveAccount(input)
	if err != nil {
		return &retry.NotFoundError{
			Message:     fmt.Sprintf("account not found in ou, %s", sourceOu),
			LastRequest: input,
		}
	}
	return nil
}
