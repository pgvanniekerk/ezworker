package worker

type Func[INPUT any] func(INPUT) error
