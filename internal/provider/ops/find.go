package ops

import (
	"context"
	"fmt"
	"github.com/aws/aws-sdk-go/aws"
	"github.com/aws/aws-sdk-go/service/organizations"
	"github.com/hashicorp/aws-sdk-go-base/tfawserr"
	"github.com/hashicorp/terraform-plugin-sdk/v2/helper/retry"
)

func FindAccountByID(ctx context.Context, conn *organizations.Organizations, id string) (*organizations.Account, error) {
	input := &organizations.DescribeAccountInput{
		AccountId: aws.String(id),
	}

	output, err := conn.DescribeAccountWithContext(ctx, input)

	if tfawserr.ErrCodeEquals(err, organizations.ErrCodeAccountNotFoundException) {
		return nil, &retry.NotFoundError{
			LastError:   err,
			LastRequest: input,
		}
	}

	if err != nil {
		return nil, err
	}

	if output == nil || output.Account == nil {
		return nil, &retry.NotFoundError{
			Message:     fmt.Sprintf("account not found, %s", id),
			LastRequest: input,
		}
	}

	if status := aws.StringValue(output.Account.Status); status == organizations.AccountStatusSuspended {
		return nil, &retry.NotFoundError{
			Message:     status,
			LastRequest: input,
		}
	}

	return output.Account, nil
}
