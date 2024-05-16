package provider

import (
	"context"
	"fmt"
	"github.com/go-pax/terraform-provider-controltower/internal/provider/ops"
	"github.com/hashicorp/terraform-plugin-sdk/v2/diag"
	"github.com/hashicorp/terraform-plugin-sdk/v2/helper/schema"
	"log"
	"sync"
)

func resourceMoveAccount() *schema.Resource {
	return &schema.Resource{
		Description: "Moves an AWS account resource.",

		CreateContext: resourceMoveAccountCreate,
		ReadContext:   resourceMoveAccountRead,
		UpdateContext: resourceMoveAccountUpdate,
		DeleteContext: resourceMoveAccountDelete,
		Importer: &schema.ResourceImporter{
			StateContext: schema.ImportStatePassthroughContext,
		},

		Schema: map[string]*schema.Schema{
			"source_ou": {
				Description: "Organizational Unit where the account is located.",
				Type:        schema.TypeString,
				Required:    true,
			},
			"destination_ou": {
				Description: "Organizational Unit where the account will be moved.",
				Type:        schema.TypeString,
				Required:    true,
			},
			"account_id": {
				Description: "ID of the AWS account.",
				Type:        schema.TypeString,
				Required:    true,
			},
		},
	}
}

var accountMutex sync.Mutex

func resourceMoveAccountCreate(ctx context.Context, d *schema.ResourceData, m interface{}) diag.Diagnostics {
	conn := m.(*AWSClient).organizationsconn

	accountMutex.Lock()
	defer accountMutex.Unlock()

	accountId := d.Get("account_id").(string)
	sourceOu := d.Get("source_ou").(string)
	destinationOu := d.Get("destination_ou").(string)

	err := ops.MoveAccount(ctx, conn, accountId, sourceOu, destinationOu)
	if err != nil {
		return diag.FromErr(err)
	}

	result, diags := waitForMove(accountId, destinationOu, m)
	if diags.HasError() {
		return diags
	}
	if !result {
		return diag.Errorf("unable to move account %s to %s", accountId, destinationOu)
	}

	return resourceMoveAccountRead(ctx, d, m)
}

func resourceMoveAccountRead(ctx context.Context, d *schema.ResourceData, m interface{}) diag.Diagnostics {
	var diags diag.Diagnostics
	conn := m.(*AWSClient).organizationsconn

	account, err := ops.FindAccountByID(ctx, conn, d.Get("account_id").(string))

	if !d.IsNewResource() && NotFound(err) {
		log.Printf("[WARN] AWS Organizations Account does not exist, removing from state: %s", d.Id())
		d.SetId("")
		return diags
	}

	if err != nil {
		diags = append(diags, diag.Diagnostic{
			Severity: diag.Error,
			Summary:  fmt.Sprintf("reading AWS Organizations Account (%s): %s", d.Id(), err),
		})
		return diags
	}

	accountId := *account.Id
	sourceOu := d.Get("source_ou").(string)
	destinationOu := d.Get("destination_ou").(string)

	_ = d.Set("account_id", accountId)
	_ = d.Set("source_ou", sourceOu)
	_ = d.Set("destination_ou", destinationOu)
	d.SetId(fmt.Sprintf("%s/%s/%s", accountId, sourceOu, destinationOu))

	return nil
}

func resourceMoveAccountUpdate(ctx context.Context, d *schema.ResourceData, m interface{}) diag.Diagnostics {
	var diags diag.Diagnostics
	conn := m.(*AWSClient).organizationsconn

	if d.HasChanges("account_id", "source_ou", "destination_ou") {
		_, err := ops.FindAccountByID(ctx, conn, d.Get("account_id").(string))

		if !d.IsNewResource() && NotFound(err) {
			log.Printf("[WARN] AWS Organizations Account does not exist, removing from state: %s", d.Id())
			d.SetId("")
			return diags
		}

		if err != nil {
			diags = append(diags, diag.Diagnostic{
				Severity: diag.Error,
				Summary:  fmt.Sprintf("reading AWS Organizations Account (%s): %s", d.Id(), err),
			})
			return diags
		}

		accountMutex.Lock()
		defer accountMutex.Unlock()

		accountId := d.Get("account_id").(string)
		sourceOu := d.Get("source_ou").(string)
		destinationOu := d.Get("destination_ou").(string)

		err = ops.MoveAccount(ctx, conn, accountId, sourceOu, destinationOu)
		if err != nil {
			return diag.Errorf("error updating account move for %s: %v", accountId, err)
		}

		result, diags := waitForMove(accountId, destinationOu, m)
		if diags.HasError() {
			return diags
		}
		if !result {
			return diag.Errorf("unable to move account %s to %s", accountId, destinationOu)
		}
	}

	return resourceMoveAccountRead(ctx, d, m)
}

func resourceMoveAccountDelete(ctx context.Context, d *schema.ResourceData, m interface{}) diag.Diagnostics {
	conn := m.(*AWSClient).organizationsconn

	accountId := d.Get("account_id").(string)
	sourceOu := d.Get("source_ou").(string)
	destinationOu := d.Get("destination_ou").(string)

	accountMutex.Lock()
	defer accountMutex.Unlock()

	result, diags := waitForMove(accountId, destinationOu, m)
	if diags.HasError() {
		return diags
	}
	if !result {
		return diag.Errorf("account %s not in correct ou, %s", accountId, destinationOu)
	}

	err := ops.MoveAccount(ctx, conn, accountId, destinationOu, sourceOu)
	if err != nil {
		return diag.Errorf("error reverting account move for %s: %v", accountId, err)
	}

	result, diags = waitForMove(accountId, sourceOu, m)
	if diags.HasError() {
		return diags
	}
	if !result {
		return diag.Errorf("unable to move account %s to %s", accountId, destinationOu)
	}

	return nil
}
