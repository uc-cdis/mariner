# Mariner: The Gen3 Workflow Execution Service

Mariner is a workflow execution service written in [Go](https://golang.org)
for running [CWL](https://www.commonwl.org) workflows on [Kubernetes](https://kubernetes.io).
Mariner's API is an implementation of the [GA4GH](https://www.ga4gh.org) 
standard [WES API](https://ga4gh.github.io/workflow-execution-service-schemas).

## How to deploy Mariner in a Gen3 environment

### Prereq's

1. Mariner depends on the [Workspace Token Service](https://github.com/uc-cdis/workspace-token-service) (WTS)
to access data from the commons.
If WTS is not already running in your environment, deploy the WTS.

2. Add the Mariner pieces to your manifest:
    1. Add [version](https://github.com/uc-cdis/gitops-dev/blob/78ce75e69c786bbdda629c6c8d76a17476c2084a/mattgarvin1.planx-pla.net/manifest.json#L19)
    2. Add [config](https://github.com/uc-cdis/gitops-dev/blob/78ce75e69c786bbdda629c6c8d76a17476c2084a/mattgarvin1.planx-pla.net/manifest.json#L183-L292)
    3. Currently mariner is not setup with network policies (this will be fixed very very soon),
    so for now in your dev or qa environment in order for mariner to work,
    [network policies must be "off"](https://github.com/uc-cdis/gitops-dev/blob/78ce75e69c786bbdda629c6c8d76a17476c2084a/mattgarvin1.planx-pla.net/manifest.json#L161)
    
### Deployment

3. Deploy the Mariner server by running `gen3 kube-setup-mariner`

### Auth and User YAML

4. Make sure you have the mariner auth scheme in your user yaml:
    1. the [policy](https://github.com/uc-cdis/commons-users/blob/a95edd2d1ac27faed2ab628280cff8923292d073/users/dev/user.yaml#L57-L60)
    2. the [resource](https://github.com/uc-cdis/commons-users/blob/a95edd2d1ac27faed2ab628280cff8923292d073/users/dev/user.yaml#L419-L420)
    3. the [role](https://github.com/uc-cdis/commons-users/blob/a95edd2d1ac27faed2ab628280cff8923292d073/users/dev/user.yaml#L577-L582)

5. Give the `mariner_admin` policy to those users who need it. ([example](https://github.com/uc-cdis/commons-users/blob/a95edd2d1ac27faed2ab628280cff8923292d073/users/dev/user.yaml#L1433))

#### Auth Note

Right now the mariner auth scheme is coarse - you 
either have access to all the API endpoints or none of them.
In order for a user (intended at this point to either be a CTDS dev or bio)
to interact with mariner, that user will need to have mariner admin privileges.

A mariner admin can do the following:
  - run workflows
  - fetch run status via runID
  - fetch run logs and output via runID
  - cancel a run that's in-progress via runID
  - query run history (i.e., fetch a list of all your runIDs)
  
### Check that it works (next)

6. You can test that Mariner is working in your environment by (TODO)

## How to use mariner (todo)

todo - one, full, worked example and flow covering all the api endpoints
