## Approach

Include the known source size in the local job state. Categorize queued local jobs before their phase, and use a single selected filter. Remove the server queue list but retain the existing aggregate statistic. Make the activity panel fill the available content width. Missing source sizes remain hidden for compatibility with older workers.

## Risks

Workers and dashboards on different versions may not provide source size; the field is optional and omitted when unknown.
