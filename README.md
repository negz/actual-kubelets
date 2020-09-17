# Actual Kubelets

Actual Kubelets (or AK for short) is a [Virtual Kubelet] provider that runs pods
on Actual Kubelets. AK joins a 'local' host cluster as a node. Any pods
scheduled to AK are proxied to a 'remote' cluster - the actual containers run on
the remote cluster.

AK is designed to support a many-to-one local-to-remote cluster relationship; it
allows many 'local' clusters to run without real nodes, instead running all of
their pods on one 'remote' cluster. Each namespace on a local cluster maps to a
unique namespace on the remote cluster, as long as there is exactly one AK node
per local cluster, and as long as all AK node names are unique relative to the
remote cluster. AK replicates all pod dependencies (i.e. secrets and configmaps)
to the remote cluster. This includes service account tokens, so any Kubernetes
controller pods scheduled to an AK node will automatically connect to the local
cluster (when they perform 'in-cluster config') despite actually running in the
remote cluster.

To try AK, first spin up a local and a remote Kubernetes cluster. Each cluster
must be able to reach the other's API server (AK is tested using GKE clusters).

```bash
# AK uses these settings to connect to the remote API server. You can generate
# a token by creating a service account in the remote cluster. The service
# account must have full access to namespaces, pods, config maps, and secrets.
REMOTE_API_SERVER_IP=10.0.0.1
REMOTE_API_TOKEN=verysecuretoken
REMOTE_API_CA_FILE=ca.crt

# AK needs this to tell the pods it runs on the remote cluster which API server
# they should connect to if they use in-cluster config to create a Kubernetes
# client (e.g. via kubectl).
LOCAL_API_SERVER_IP=10.0.0.1

# Install AK to the 'local' Kubernetes server.
helm install example helm/ \
    --namespace kube-system \
    --set local.apiserverHost=${LOCAL_API_SERVER_IP} \
    --set remote.apiserverHost=${REMOTE_API_SERVER_IP} \
    --set remote.token=${REMOTE_API_TOKEN} \
    --set-file remote.caData=${REMOTE_API_CA_FILE}

# Now install something to try out. You'll probably want to cordon any non-AK
# nodes to ensure your pods are scheduled to AK.
kubectl create namespace crossplane-system
helm install crossplane --namespace crossplane-system crossplane-alpha/crossplane

# You should see your pods running on AK in the local API server
kubectl -n crossplane-system get po -o wide
NAME                                          READY   STATUS      RESTARTS   AGE   IP          NODE            NOMINATED NODE   READINESS GATES
crossplane-684d498858-frd4t                   1/1     Running     0          34m   10.16.1.9   vk-ak-example   <none>           <none>
crossplane-package-manager-6598cb864b-88l27   1/1     Running     0          34m   10.16.2.8   vk-ak-example   <none>           <none>
provider-gcp-controller-c45fb56b4-v7jmd       1/1     Running     0          26m   10.16.0.6   vk-ak-example   <none>           <none>
provider-gcp-gtzz7                            0/1     Completed   0          26m   10.16.0.5   vk-ak-example   <none>           <none>

# Logs and execution work just fine.
$ kubectl -n crossplane-system logs --tail 1 crossplane-684d498858-frd4t
2020-09-17T09:33:50.965Z        INFO    controller-runtime.controller   Starting workers        {"controller": "apiextensions/infrastructurepublication.apiextensions.crossplane.io", "worker count": 5}

# However your pods are actually running in the remote cluster! This part
# assumes 'remote.yaml' is a kubeconfig for the remote API server.
kubectl --kubeconfig=remote.yaml get namespace -l actual.vk/node-name=vk-ak-example,actual.vk/namespace=crossplane-system
NAME                             STATUS   AGE
vk-ak-example-d05fe343c4951ecb   Active   73m

kubectl --kubeconfig=remote.yaml -n vk-ak-example-d05fe343c4951ecb get po -o wide
NAME                                          READY   STATUS      RESTARTS   AGE   IP          NODE                                         NOMINATED NODE   READINESS GATES
crossplane-684d498858-frd4t                   1/1     Running     0          41m   10.16.1.9   gke-remote-host-default-pool-307c8cba-t9wf   <none>           <none>
crossplane-package-manager-6598cb864b-88l27   1/1     Running     0          41m   10.16.2.8   gke-remote-host-default-pool-307c8cba-rtcg   <none>           <none>
provider-gcp-controller-c45fb56b4-v7jmd       1/1     Running     0          33m   10.16.0.6   gke-remote-host-default-pool-307c8cba-f676   <none>           <none>
provider-gcp-gtzz7                            0/1     Completed   0          34m   10.16.0.5   gke-remote-host-default-pool-307c8cba-f676   <none>           <none>
```

[Virtual Kubelet]: https://virtual-kubelet.io/
