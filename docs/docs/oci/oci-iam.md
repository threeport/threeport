# OCI IAM Permissions for Threeport

This guide provides instructions for setting up the necessary IAM permissions in Oracle Cloud Infrastructure (OCI) to deploy and manage Threeport. Instead of using the built-in `Administrator` policy, we'll create a more restricted set of permissions that follows the principle of least privilege.

## Prerequisites

- OCI CLI installed and configured
- Access to an OCI tenancy with administrative privileges (needed only for initial setup)

## Environment Setup

Before running the CLI commands, you'll need to set up some environment variables. You can get these values from the OCI Console:

1. **Tenancy OCID**: Found in the OCI Console under Administration > Tenancy Details
2. **Compartment OCID**: Found in the OCI Console under Identity > Compartments
3. **User OCID**: Found in the OCI Console under Identity > Users

Set these environment variables:

```bash
# Required environment variables
export TENANCY_OCID="ocid1.tenancy.oc1..exampleuniqueID"
export COMPARTMENT_ID="ocid1.compartment.oc1..exampleuniqueID"
export USER_OCID="ocid1.user.oc1..exampleuniqueID"

# Optional: Set the region if different from your default
export OCI_REGION="us-ashburn-1"  # or your preferred region
```

You can verify your environment variables are set correctly:

```bash
echo "Tenancy OCID: $TENANCY_OCID"
echo "Compartment ID: $COMPARTMENT_ID"
echo "User OCID: $USER_OCID"
echo "Region: $OCI_REGION"
```

## Create Required Groups and Policies

First, let's create a group for Threeport administrators:

```bash
# Create the Threeport administrators group
GROUP_ID=$(oci iam group create \
    --name threeport-admins \
    --description "Group for Threeport administrators" \
    --query 'data.id' \
    --raw-output)

echo "Created group with ID: $GROUP_ID"
```

Now, let's create the necessary policies. We'll create them in the root compartment:

```bash
# Create policy for OKE cluster management
POLICY_ID=$(oci iam policy create \
    --name threeport-oke-policy \
    --description "Policy for managing OKE clusters for Threeport" \
    --compartment-id $TENANCY_OCID \
    --statements '[
        "Allow group threeport-admins to inspect compartments in compartment id '${TENANCY_OCID}'",
        "Allow group threeport-admins to manage clusters in compartment id '${TENANCY_OCID}'",
        "Allow group threeport-admins to manage virtual-network-family in compartment id '${TENANCY_OCID}'",
        "Allow group threeport-admins to manage compute-family in compartment id '${TENANCY_OCID}'",
        "Allow group threeport-admins to manage volume-family in compartment id '${TENANCY_OCID}'",
        "Allow group threeport-admins to manage load-balancers in compartment id '${TENANCY_OCID}'",
        "Allow group threeport-admins to use vnics in compartment id '${TENANCY_OCID}'",
        "Allow group threeport-admins to use network-security-groups in compartment id '${TENANCY_OCID}'",
        "Allow group threeport-admins to use private-ips in compartment id '${TENANCY_OCID}'",
        "Allow group threeport-admins to manage public-ips in compartment id '${TENANCY_OCID}'",
        "Allow group threeport-admins to manage object-family in compartment id '${TENANCY_OCID}'",
        "Allow group threeport-admins to manage tag-namespaces in compartment id '${TENANCY_OCID}'",
        "Allow group threeport-admins to manage tag-defaults in compartment id '${TENANCY_OCID}'",
        "Allow group threeport-admins to use tag-namespaces in compartment id '${TENANCY_OCID}'",
        "Allow group threeport-admins to inspect availability-domains in compartment id '${TENANCY_OCID}'"
    ]')

echo "Created policy with ID: $POLICY_ID"
```

## Required Permissions Breakdown

The policy above grants the following permissions:

1. **OKE Cluster Management**
   - Create and manage OKE clusters and node pools (`cluster-family`)
   - Inspect compartments for resource organization
   - Scale clusters and node pools

2. **Networking**
   - Create and manage VCNs, subnets, gateways, route tables, and security lists (`virtual-network-family`)
   - Use and manage network security groups
   - Use VNICs (Virtual Network Interface Cards)
   - Use private IPs and manage public IPs
   - Configure load balancers

3. **Compute Resources**
   - Manage compute instances for worker nodes (`instance-family`)
   - Manage block volumes for persistent storage (`volume-family`)
   - Manage instance configurations

4. **Object Storage**
   - Create and manage buckets
   - Manage objects and lifecycle policies (`object-family`)

5. **Identity and Tagging**
   - Manage and use tag namespaces for resource organization
   - Manage tag defaults
   - Inspect availability domains for resource placement
   - Read OCI services information (required for service gateways)

## Add Users to the Group

To add users to the Threeport administrators group:

```bash
# Add a user to the Threeport administrators group
oci iam group add-user \
    --group-id $GROUP_ID \
    --user-id $USER_OCID

echo "Added user $USER_OCID to group $GROUP_ID"
```

## Verify Permissions

To verify that a user has the correct permissions:

```bash
# List all policies for the compartment
oci iam policy list \
    --compartment-id $COMPARTMENT_ID

# List all groups
oci iam group list

# List users in the Threeport administrators group
oci iam group list-users \
    --group-id $GROUP_ID
```

## Best Practices

1. **Compartment Organization**: Consider creating a dedicated compartment for Threeport resources to better manage access and costs.

2. **Regular Audits**: Periodically review the permissions to ensure they remain appropriate for your needs.

3. **User Management**: Remove users from the group when they no longer need access to Threeport resources.

4. **Policy Updates**: Keep the policies updated as Threeport's requirements evolve.

## Troubleshooting

If you encounter permission issues:

1. Verify the user is in the correct group:
```bash
oci iam group list-users --group-id $GROUP_ID
```

2. Check the effective policies for the user:
```bash
oci iam policy list --compartment-id $COMPARTMENT_ID
```

3. Verify the policy statements are correctly formatted and applied to the right compartment.

## Next Steps

After setting up the IAM permissions, you can proceed with the [Threeport installation on OCI](../install/install-threeport-oci.md).