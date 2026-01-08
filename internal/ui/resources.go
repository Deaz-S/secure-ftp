// Package ui provides embedded resources for the application.
package ui

import (
	"encoding/base64"

	"fyne.io/fyne/v2"
)

// Icon PNG data in base64
const iconBase64 = "iVBORw0KGgoAAAANSUhEUgAAAEAAAABACAYAAACqaXHeAAACVElEQVR4nO2bPVLDMBCFBUNBERqGC+CWOyR1isxAnwuEA8EF6GEmBbRwB1pzgUwaKOig2kSW9bfySivJ/qrgCOm955UsYyzExLg5STnY9cP3n2/br/uLJNqiDoIx7CJWIFE6pTSuQh0EWWcxTZugCOOUQgiHeapxByXIZVxHaDUEV0BO5oUI1xMUQG7mgRBd6AByNQ9g9aECyN08gNHpHUAp5gFfvV4BlGYe8NHtDKBU84BLvzWA0s0DNh8kO8GSMQZQy9kHTH60AdRmHtD5Gv0UOMP+QrtYkw3evD+R9RVKrwJSlj9lmL6o/tinAEcIMp0AuBa/1CHIPtFrQCzaxVqI3RtJX83nzrstPgAikbnAvgZwcwig1s2PCfA7VUDqAVf7pVjtl+jvYuEMoN3MUuiIhku/NYDSzQM2H8bLILV5tbRtpQ7fbS9fycZvNzPRPP70jmsroJYzr6Lz1asAXaPusd/jx49zEmEkzI+62rm5mVoJo78M9ioA0pHPupxYe3NFNvj29vnwefVyN6wzqRrlewGTD8BYAbrGlMjmdT9TY/JzCED3eDl2CKnQ+QC/zjWg9BBc+tkWQXXOD14DAmH9gwiXaZlOBaT63zxuZJ+j3wf0Aqi9ClR/ydYAyhsbSqYpoDtY6zTQ+TJWQG0hmPyg1wDMQ4cSsK4BtVSBzYdzESw9BJd+r6tAqSH46Pa+DJYWgq9e1D6glBAwOtEbodxDwOoL2gnmGkKIruCtcG4hhOohMcH5aH3oiSC5GeKqBopxp/cGKTtTGe2boyZyfHd49PwD6m7n3EGi1OYAAAAASUVORK5CYII="

// AppIcon is the application icon resource.
var AppIcon fyne.Resource

func init() {
	iconData, _ := base64.StdEncoding.DecodeString(iconBase64)
	AppIcon = &fyne.StaticResource{
		StaticName:    "icon.png",
		StaticContent: iconData,
	}
}
