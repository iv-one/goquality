package check

// Grade is a letter grade between A+ (highest) and F (lowest).
type Grade string

// Grades, from best to worst.
const (
	GradeAPlus Grade = "A+"
	GradeA     Grade = "A"
	GradeB     Grade = "B"
	GradeC     Grade = "C"
	GradeD     Grade = "D"
	GradeE     Grade = "E"
	GradeF     Grade = "F"
)

// GradeFromPercentage maps a 0..100 score to a grade, using Go Report Card's
// thresholds.
func GradeFromPercentage(percentage float64) Grade {
	switch {
	case percentage > 90:
		return GradeAPlus
	case percentage > 80:
		return GradeA
	case percentage > 70:
		return GradeB
	case percentage > 60:
		return GradeC
	case percentage > 50:
		return GradeD
	case percentage > 40:
		return GradeE
	default:
		return GradeF
	}
}
