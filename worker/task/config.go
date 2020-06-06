package task

import "transcoder/model"

type AcceptedJobs []model.JobType

func (A AcceptedJobs) IsAccepted(jobType model.JobType) bool{
	for _,j := range A {
		if j == jobType{
			return true
		}
	}
	return false
}
type Config struct {
	TemporalPath     string
	WorkerName       string
	WorkerThreads    int
	AcceptedJobs     AcceptedJobs
	WorkerEncodeJobs int
	WorkerPGSJobs    int
	WorkerPriority   int
}
