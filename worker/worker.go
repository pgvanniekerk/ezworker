package worker

type Worker[INPUT any] interface {
	Execute(INPUT) error
}
