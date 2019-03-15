# Developer setup for working on cluster-api-provider-libvirt

Firstly you should have a Openshift 4.0 cluster up and running. This is already documented elsewhere so I won't get
into that. Then you need to build a custom image for cluster-api-provider-libvirt:

```
# In ${GOPATH}/src/github.com/openshift/
git clone git@github.com:openshift/cluster-api-provider-libvirt.git
cd cluster-api-provider-libvirt
# Make any changes you need to in the source code
sudo docker build -t ${YOUR_DOCKER_NS}/capl:test .
sudo docker login # If not already done
sudo docker push ${YOUR_DOCKER_NS}/capl:test
```

Let's avoid having to specify namespace all the time to `oc`:

```
oc project openshift-machine-api
```

## Deploying the custom image

Before you can deploy your custom image, you need to disable CVO:

```
oc scale --replicas 0 deployments/cluster-version-operator -n openshift-cluster-version
```

There are at least two paths to deploying the image, the intrusive one and less-intrusive one.

### The intrusive method

This one means you also disable MAO:

```
oc scale --replicas 0 deployments/machine-api-operator
```

Now you just edit the `clusterapi-manager-controllers` deployment and tell it to use your build image:

```
oc edit deployments clusterapi-manager-controllers
```

It's the `image` under `./manager` and `/machine-controller-manager` commands that you need to modify to
`${YOUR_DOCKER_NS}/capl:test`.

Wait a bit and your new image will be deployed. You can monitor the status with ` oc get pods`. You can also see the
logs with `oc logs deploy/clusterapi-manager-controllers -c machine-controller | less`.


### The less intrusive method:

Instead of disabling MAO, you just update the relevant configmaps:

```
oc edit configmaps/machine-api-operator-images
```

and override the `clusterAPIControllerLibvirt` to your image. Then reset the relevant controller to make it get the new
image:

```
oc scale --replicas 0 deployments/clusterapi-manager-controllers
oc scale --replicas 1 deployments/clusterapi-manager-controllers
```

Wait a bit and your new image will be deployed. You can monitor in the same way as the other method.

## Testing

So far you've only seen your new modified fancy provider coming up successfully. Let's make it work a bit by scaling up
the worker nodes:

```
oc get machinesets
# The output will tell you the name of the worker machineset
# Let's assume it's "test1-wk7xq-worker-0" below
oc scale --replicas 2 machinesets/test1-wk7xq-worker-0
```

You should be immediately able to see machines coming up with `oc get machines` but it'll take a bit before machine is
ready. You can monitor that using `oc get machinesets`. Also you can see the logs using the command mentioned
previously to see what's happening.
