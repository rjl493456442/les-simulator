# Light-ethereum-service protocol simulator

## Installation

```shell=
go get install https://github.com/rjl493456442/les-simulator
```


## Build your own test simulation

In the `cmd` directory there are two examples.

- les-example: It's the easiest test scenario which connects a single server and client together.
- lespay: It's the testing scenario that the lottery payment is enabled.

In order to build your test simuation, you can copy the `les-example` and customize the `cluster` configuration. Don't forget to replace your `go-ethereum` library if the testing functionality is not on the default library. 


###### tags: `LES Protocol` `Simulation` `Testing`
