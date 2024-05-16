package provider

import (
	"context"
	"fmt"
	"github.com/aws/aws-sdk-go/aws"
	"github.com/aws/aws-sdk-go/service/organizations"
	"github.com/aws/aws-sdk-go/service/servicecatalog"
	"github.com/go-pax/terraform-provider-controltower/internal/provider/ops"
	"github.com/hashicorp/terraform-plugin-sdk/v2/diag"
	"time"
)

func waitForMove(accountId string, parentOu string, m interface{}) (bool, diag.Diagnostics) {
	conn := m.(*AWSClient).organizationsconn
	input := &organizations.ListAccountsForParentInput{ParentId: aws.String(parentOu)}

	for {
		status, err := conn.ListAccountsForParent(input)
		if err != nil {
			return false, diag.Errorf("error reading OU status of account %s: %v", accountId, err)
		}

		for _, account := range status.Accounts {
			id := *account.Id
			if id == accountId {
				return true, nil
			}
		}

		time.Sleep(5 * time.Second)
	}
}

func getOrganizationalUnits(OU *organizations.OrganizationalUnit, conn *organizations.Organizations) ([]*organizations.OrganizationalUnit, error) {
	var ouSlice []*organizations.OrganizationalUnit
	var nextToken *string

	for {
		OUs, err := conn.ListOrganizationalUnitsForParent(&organizations.ListOrganizationalUnitsForParentInput{
			ParentId:  OU.Id,
			NextToken: nextToken,
		})
		if err != nil {
			return nil, err
		}

		for childOU := 0; childOU < len(OUs.OrganizationalUnits); childOU++ {
			ouSlice = append(ouSlice, OUs.OrganizationalUnits[childOU])
		}

		if OUs.NextToken == nil {
			break
		}
		nextToken = OUs.NextToken
	}

	return ouSlice, nil
}

func getOrganizationalUnitsRecursive(ou *organizations.OrganizationalUnit, conn *organizations.Organizations) ([]*organizations.OrganizationalUnit, error) {
	var ous []*organizations.OrganizationalUnit

	currentOUs, err := getOrganizationalUnits(ou, conn)
	if err != nil {
		return nil, err
	}

	for _, currentOU := range currentOUs {
		ous = append(ous, currentOU)

		deepOus, _ := getOrganizationalUnitsRecursive(currentOU, conn)
		ous = append(ous, deepOus...)
	}

	return ous, nil
}

func findAccountOrganizationalUnit(accountId string, conn *organizations.Organizations) (string, string, diag.Diagnostics) {
	ous := make(map[string]string)

	input := &organizations.ListRootsInput{}
	status, err := conn.ListRoots(input)
	if err != nil {
		return "", "", diag.Errorf("error reading OU root account %s: %v", accountId, err)
	}

	root := status.Roots[0]
	ous[*root.Id] = *root.Name

	ou := organizations.OrganizationalUnit{
		Arn:  root.Arn,
		Id:   root.Id,
		Name: root.Name,
	}

	ouSlice, err := getOrganizationalUnitsRecursive(&ou, conn)
	if err != nil {
		return "", "", diag.FromErr(err)
	}

	for _, ou := range ouSlice {
		ous[*ou.Id] = *ou.Name
	}
	//
	//var nextToken *string
	//for {
	//	ouOutput, err := conn.ListOrganizationalUnitsForParent(&organizations.ListOrganizationalUnitsForParentInput{
	//		ParentId:  root.Id,
	//		NextToken: nextToken,
	//	})
	//	if err != nil {
	//		return "", "", diag.Errorf("error reading OUs under parent %s: %v", *root.Id, err)
	//	}
	//
	//	for _, ou := range ouOutput.OrganizationalUnits {
	//		ous[*ou.Id] = *ou.Name
	//	}
	//
	//	if ouOutput.NextToken == nil {
	//		break
	//	}
	//	nextToken = ouOutput.NextToken
	//}

	for ou, name := range ous {
		var nextToken *string
		for {
			aOutput, err := conn.ListAccountsForParent(&organizations.ListAccountsForParentInput{
				ParentId:  aws.String(ou),
				NextToken: nextToken,
			})
			if err != nil {
				return "", "", diag.Errorf("error reading accounts under OU %s: %v", ou, err)
			}
			for _, account := range aOutput.Accounts {
				if accountId == *account.Id {
					return ou, name, nil
				}
			}

			if aOutput.NextToken == nil {
				break
			}
			nextToken = aOutput.NextToken
		}
	}

	return "", "", diag.Errorf("no OU found for account %s.", accountId)
}

func findAccountWithOrganizationalUnit(ctx context.Context, id string, m interface{}) (*organizations.Account, string, string, diag.Diagnostics) {
	conn := m.(*AWSClient).organizationsconn

	account, err := ops.FindAccountByID(ctx, conn, id)
	if err != nil {
		return nil, "", "", diag.FromErr(err)
	}

	ou, ouName, diags := findAccountOrganizationalUnit(*account.Id, conn)
	if diags.HasError() {
		return nil, "", "", diags
	}
	if ou == "" {
		return nil, "", "", diag.Errorf("unable to find ou for account %s", *account.Id)
	}
	return account, ou, ouName, nil
}

func findServiceCatalogAccountProductId(conn *servicecatalog.ServiceCatalog) (*string, *string, error) {
	products, err := conn.SearchProducts(&servicecatalog.SearchProductsInput{
		Filters: map[string][]*string{"FullTextSearch": {aws.String("AWS Control Tower Account Factory")}},
	})
	if err != nil {
		return nil, nil, fmt.Errorf("error occured while searching for the account product: %v", err)
	}
	if len(products.ProductViewSummaries) != 1 {
		return nil, nil, fmt.Errorf("unexpected number of search results: %d", len(products.ProductViewSummaries))
	}

	productId := products.ProductViewSummaries[0].ProductId

	artifacts, err := conn.ListProvisioningArtifacts(&servicecatalog.ListProvisioningArtifactsInput{
		ProductId: productId,
	})
	if err != nil {
		return productId, nil, fmt.Errorf("error listing provisioning artifacts: %v", err)
	}

	// Try to find the active (which should be the latest) artifact.
	var artifactID *string
	for _, artifact := range artifacts.ProvisioningArtifactDetails {
		if *artifact.Active {
			artifactID = artifact.Id
			break
		}
	}
	if artifactID == nil {
		return productId, nil, fmt.Errorf("could not find the provisioning artifact ID")
	}

	return productId, artifactID, nil
}

func findParentOrganizationalUnit(conn *organizations.Organizations, identifier string) (*organizations.OrganizationalUnit, error) {
	parents, err := conn.ListParents(&organizations.ListParentsInput{
		ChildId: aws.String(identifier),
	})
	if err != nil {
		return nil, fmt.Errorf("error reading parents for %s: %v", identifier, err)
	}

	var parentOuId string
	for _, v := range parents.Parents {
		if *v.Type == organizations.ParentTypeOrganizationalUnit {
			parentOuId = *v.Id
			break
		}
	}
	if parentOuId == "" {
		return nil, fmt.Errorf("no OU parent found for %s", identifier)
	}

	ou, err := conn.DescribeOrganizationalUnit(&organizations.DescribeOrganizationalUnitInput{
		OrganizationalUnitId: aws.String(parentOuId),
	})
	if err != nil {
		return nil, fmt.Errorf("error describing parent OU %s: %v", parentOuId, err)
	}

	return ou.OrganizationalUnit, nil
}

func findParentOrganizationRootId(conn *organizations.Organizations, identifier string) (string, error) {
	parents, err := conn.ListParents(&organizations.ListParentsInput{
		ChildId: aws.String(identifier),
	})
	if err != nil {
		return "", fmt.Errorf("error reading parents for %s: %v", identifier, err)
	}

	for _, v := range parents.Parents {
		if *v.Type == organizations.ParentTypeRoot {
			return *v.Id, nil
		}
	}

	return "", fmt.Errorf("no organization root parent found for %s", identifier)
}

func toOrganizationsTags(tags map[string]interface{}) []*organizations.Tag {
	result := make([]*organizations.Tag, 0, len(tags))

	for k, v := range tags {
		tag := &organizations.Tag{
			Key:   aws.String(k),
			Value: aws.String(v.(string)),
		}

		result = append(result, tag)
	}

	return result
}

func fromOrganizationTags(tags []*organizations.Tag) map[string]*string {
	m := make(map[string]*string, len(tags))

	for _, tag := range tags {
		m[aws.StringValue(tag.Key)] = tag.Value
	}

	return m
}

func updateAccountTags(conn *organizations.Organizations, identifier string, oldTags interface{}, newTags interface{}) error {
	oldTagsMap := oldTags.(map[string]interface{})
	newTagsMap := newTags.(map[string]interface{})

	if removedTags := removedTags(oldTagsMap, newTagsMap); len(removedTags) > 0 {
		input := &organizations.UntagResourceInput{
			ResourceId: aws.String(identifier),
			TagKeys:    aws.StringSlice(keys(removedTags)),
		}

		_, err := conn.UntagResource(input)

		if err != nil {
			return fmt.Errorf("error untagging resource (%s): %w", identifier, err)
		}
	}

	if updatedTags := updatedTags(oldTagsMap, newTagsMap); len(updatedTags) > 0 {
		input := &organizations.TagResourceInput{
			ResourceId: aws.String(identifier),
			Tags:       toOrganizationsTags(updatedTags),
		}

		_, err := conn.TagResource(input)

		if err != nil {
			return fmt.Errorf("error tagging resource (%s): %w", identifier, err)
		}
	}

	return nil
}

func removedTags(oldTagsMap map[string]interface{}, newTagsMap map[string]interface{}) map[string]interface{} {
	result := map[string]interface{}{}

	for k, v := range oldTagsMap {
		if _, ok := newTagsMap[k]; !ok {
			result[k] = v
		}
	}

	return result
}

func updatedTags(oldTagsMap map[string]interface{}, newTagsMap map[string]interface{}) map[string]interface{} {
	result := map[string]interface{}{}

	for k, newV := range newTagsMap {
		if oldV, ok := oldTagsMap[k]; !ok || oldV != newV {
			result[k] = newV
		}
	}

	return result
}

func keys(value map[string]interface{}) []string {
	keys := make([]string, 0, len(value))
	for k := range value {
		keys = append(keys, k)
	}

	return keys
}
