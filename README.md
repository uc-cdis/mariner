# mariner: the Gen3 workflow execution service

Mariner is a workflow execution service written in [Go](https://golang.org)
for running [CWL](https://www.commonwl.org) workflows on [Kubernetes](https://kubernetes.io).
Mariner's API is an implementation of the [GA4GH](https://www.ga4gh.org) 
standard [WES API](https://ga4gh.github.io/workflow-execution-service-schemas).

## How to deploy mariner in a gen3 environment

### Prereq's

1. Mariner depends on the workspace-token-service to access data from the commons.
If WTS is not already running in your environment, deploy the WTS.

2. Add the Mariner pieces to your manifest:
    1. add [version](https://github.com/uc-cdis/gitops-dev/blob/78ce75e69c786bbdda629c6c8d76a17476c2084a/mattgarvin1.planx-pla.net/manifest.json#L19)
    2. add [config](https://github.com/uc-cdis/gitops-dev/blob/78ce75e69c786bbdda629c6c8d76a17476c2084a/mattgarvin1.planx-pla.net/manifest.json#L183-L292)
    3. currently mariner is not setup with network policies (this will be fixed very very soon),
    so for now in your dev or qa environment in order for mariner to work,
    [network policies must be "off"](https://github.com/uc-cdis/gitops-dev/blob/78ce75e69c786bbdda629c6c8d76a17476c2084a/mattgarvin1.planx-pla.net/manifest.json#L161)
    
### Deployment (next)

3. Deploy the Mariner server by running `gen3 kube-setup-mariner`.

at this point, the mariner server is running in your environment,
but you don't have authZ to do anything with it
we can fix that by granting you mariner admin privileges

! see also: the secret situation! AWS user creds, see Reuben's note

### Auth and User YAML (todo)

4. add 'mariner_admin' to your policy list in the user.yaml for your environment
-- like so: https://github.com/uc-cdis/commons-users/blob/master/users/dev/user.yaml#L1430-L1433
  i) if the mariner auth scheme isn't already in the user.yaml for your environment,
  -- you'll need to add the following sections to your user.yaml:
    - policy: https://github.com/uc-cdis/commons-users/blob/master/users/dev/user.yaml#L57-L60
    - resource: https://github.com/uc-cdis/commons-users/blob/master/users/dev/user.yaml#L419-L420
    - role: https://github.com/uc-cdis/commons-users/blob/master/users/dev/user.yaml#L577-L582
  (REVISE)(note: currently the mariner auth scheme exists only in the dev user.yaml)

now that you're an admin, you can 
  i) run workflows
  ii) fetch run status via runID
  iii) fetch run logs and output via runID
  iv) cancel a run that's in-progress via runID
  v) query your run history (get back a list of all your runIDs)
  
### Check that it works (todo)

5. You can test that Mariner is working in your environment by (TODO)

## How to use mariner (todo)

todo - one, full, worked example and flow covering all the api endpoints
