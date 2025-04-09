# operator


## Description

A basic operator to watch custom resrouce and create pod managed by that resource.

## Getting Started

### Prerequisites
- docker version 17.03+.
- kubectl version v1.11.3+.
- Access to a Kubernetes v1.11.3+ cluster.

### Deploy 

Run `kubectl kustomize config/default | kubectl apply -f -` 

to deploy operator to cluster in current kubectl context.

Verify that the dummy-operator is up and running:

```
$ kubectl get deployment -n dummy-operator-system
NAME                          READY   UP-TO-DATE   AVAILABLE   AGE
operator-controller-manager   1/1     1            1           15s
```
It is configure to deploy the operator image `ghcr.io/junaidk/dummy-operator:latest`

### Deploy CR

Sample CR is avialable at `config/samples/tools_v1alpha1_dummy.yaml`

Create the CR:

`kubectl apply -f config/samples/tools_v1alpha1_dummy.yaml`

Ensure that the operator creates the pod for the sample CR:

```
$ kubectl get po
NAME           READY   STATUS    RESTARTS   AGE
dummy-sample   1/1     Running   0          8s
```

Check the pods and CR status to confirm the status is updated:

```
$ kubectl describe dummy
Name:         dummy-sample
Namespace:    default
Labels:       app.kubernetes.io/managed-by=kustomize
              app.kubernetes.io/name=operator
Annotations:  <none>
API Version:  tools.interview.com/v1alpha1
Kind:         Dummy
Metadata:
  Creation Timestamp:  2025-04-09T14:30:31Z
  Finalizers:
    tools.interview.com/finalizer
  Generation:        1
  Resource Version:  69003
  UID:               aa156d8b-7b46-47c2-b39b-d014d6633ff1
Spec:
  Message:  hello
Status:
  Pod Status:  Running
  Spec Echo:   hello
Events:        <none>
```

### Cleanup

`kubectl delete -f config/samples/tools_v1alpha1_dummy.yaml`

`kubectl kustomize config/default | kubectl delete --ignore-not-found=$(ignore-not-found) -f -`