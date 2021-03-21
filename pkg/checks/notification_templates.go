package checks

const stateChangeTemplate = `{
	"blocks": [
		{
			"type": "header",
			"text": {
				"type": "plain_text",
				"text": "%s: %s → %s",
				"emoji": true
			}
		},
		{
			"type": "context",
			"elements": [
				{
					"type": "mrkdwn",
					"text": %s
				}
			]
		},
		{
			"type": "context",
			"elements": [
				{
					"type": "mrkdwn",
					"text": %s
				}
			]
		},
		{
			"type": "context",
			"elements": [
				{
					"type": "mrkdwn",
					"text": %s
				}
			]
		}
	]
}`
