package main

import (
	"time"

	"github.com/filecoin-project/go-state-types/big"
)

func getMaxAllowanceForGithub(githubAccount string, filecoinAddress string) (big.Int, error) {
	// Check GitHub account activity
	activityCheck, err := checkGithubAccountActivity(githubAccount, 3)
	if err != nil {
		return big.Zero(), err
	}

	// Get Filecoin deals multiplier
	dealsMultiplier, err := getFilecoinDealsMultiplier(filecoinAddress)
	if err != nil {
		return big.Zero(), err
	}

	// Calculate allowance
	score := env.BaseAllowanceBytes
	if activityCheck {
		score = big.Mul(score, big.NewInt(2))
	}
	if dealsMultiplier != 1 {
		score = big.Mul(score, big.NewInt(int64(dealsMultiplier)))
	}
	return score, nil
}

/*
 * Returns true when there was public activity in the GitHub account for the past X months.
 * Also returns true when there was too much recent activity and the returned events from
 * GitHub do not reach back far enought to give a proper evaluation.
 */
func checkGithubAccountActivity(githubAccount string, months int) (bool, error) {
	// Get event dates from the GitHub account
	dates, err := getGitHubEventDates(githubAccount)
	if err != nil {
		return false, err
	}

	// Evaluate account activity
	dateCount := len(dates)
	githubMaxDateCount := 300
	activityCheck, enoughDates := hasDateInEachMonthBefore(months, dates)
	historyInsufficient := dateCount == githubMaxDateCount && !enoughDates

	// Github limits the maximum event history. When we have
	// the maximum amount of events but not enough data to go
	// back far enough, we give the user the benefit of the doubt.
	return activityCheck || historyInsufficient, nil
}

/*
 * Returns a multiplier for the max allowance based on the
 * verified deal count for the supplied filecoin address
 */
func getFilecoinDealsMultiplier(filecoinAddress string) (int, error) {
	// Get amount or verified deals for Filecoin address
	dealCount, err := getVerifiedDealCount(filecoinAddress)
	if err != nil {
		return 0, err
	}

	// Return multiplier
	if dealCount > 100 {
		return 8, nil
	}
	if dealCount > 10 {
		return 4, nil
	}
	if dealCount > 0 {
		return 2, nil
	}
	return 1, nil
}

/*
 * Checks for an X amount of months before today, whether the supplied dates
 * contain a date between the start- and endtime of each of the months. For example,
 * when today is 2022-03-23, a date need to be present in all the following months:
 * 2022-02-23 to 2022-03-23,
 * 2022-01-23 to 2022-02-23 and
 * 2021-12-23 to 2022-01-23
 * The second return value indicates whether the history of the dates went
 * back far enough to check for the presence of a date in each of the months.
 */
func hasDateInEachMonthBefore(months int, dates []time.Time) (bool, bool) {
	monthEndTime := time.Now()
	for i := 0; i < months; i++ {
		// Check whether a date exists in the month before monthEndTime
		hasDate, enoughDates := hasDateInMonth(monthEndTime, dates)
		// Return instantly when a date is not found and
		// indicate whether we had enough dates to evaluate
		if !hasDate {
			return false, enoughDates
		}
		// Go back one month before checking again
		monthEndTime = monthEndTime.AddDate(0, -1, 0)
	}
	return true, true
}

/*
 * Checks whether a date in the supplied dates falls in the month ending at the supplied
 * endTime. The second return value indicates whether there were enough dates. When none
 * of the dates fall in or before the given month, more history might be required.
 */
func hasDateInMonth(endTime time.Time, dates []time.Time) (bool, bool) {
	startTime := endTime.AddDate(0, -1, 0)
	enoughDates := false
	for _, date := range dates {
		// Return instantly when date falls in the month
		if date.After(startTime) && date.Before(endTime) {
			return true, true
		}
		// If a date is before the start time
		// the dataset should have been big enough
		if date.Before(startTime) {
			enoughDates = true
		}
	}
	return false, enoughDates
}
