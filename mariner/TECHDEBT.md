# Tech Debt

### resolveExpression ${} parsing

Created: 9/30/2021

Issue: When parsing ${} expressions, we assume it is only 1 ${} and do not ensure that there are multiple js expressions

Why this occured: This was a stopgap to get the pre genesis workflow working, previosuly mariner did not support evaluation of ${} expressions

Imapct: We won't be able to correctly parse and evaluate ${} ${} .... ${}

Planned Resolution: As part of PXP-8786, we will refactor how js expressions are evaluated and will fix this issue.
