package workerpool

type WorkFunc[INPUT any] func(INPUT) error
