package provider

import (
	"testing"
)

func TestAccountResource(t *testing.T) {

	//testProfile := os.Getenv("AWS_PROFILE")
	/*
		t.Run("testing move_account resource", func(t *testing.T) {

			accoundId := "533267287278"
			destinationOu := "ou-i15j-k29zevig"
			sourceOu := "r-i15j"

			config := fmt.Sprintf(`
				provider "controltower" {
				  region = "us-east-1"
				  //assume_role {
					//role_arn = "arn:aws:iam::650007492008:role/OrganizationAccountAccessRole"
				  //}
				}

				resource "controltower_move_account" "test" {
				  account_id     = "%s"
				  destination_ou = "%s"
				  source_ou      = "%s"
				}
			`, accoundId, destinationOu, sourceOu)

			resource.UnitTest(t, resource.TestCase{
				ProviderFactories: providerFactories,
				Steps: []resource.TestStep{
					{
						Config: config,
						Check: resource.ComposeTestCheckFunc(
							func(s *terraform.State) error {
								rs := s.RootModule().Resources["controltower_move_account.test"]
								att := rs.Primary.Attributes["id"]
								if att == "" {
									return fmt.Errorf("expected 'id' to have a value")
								}
								return nil
							},
						),
					},
				},
			})
		})*/
}
