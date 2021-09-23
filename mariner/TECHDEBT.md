# Tech Debt

### resolveExpression ${} parsing

Created: 9/30/2021

Issue: right now we are not able to parse a combination of ${} and $() expressions. We are only able to parse either only ${} or $()

Why this occured: This was a stopgap to get the pre genesis workflow working, previosuly mariner did not support evaluation of ${} expressions

Imapct: We won't be able to correctly parse and evaluate for example ${....} .... $()

Planned Resolution: As part of PXP-8786, we will refactor how js expressions are evaluated and will fix this issue.
