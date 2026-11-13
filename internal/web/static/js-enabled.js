// Tells GOV.UK Frontend's styles that JavaScript is available. GOV.UK's page template does this
// with an inline script, which the Content Security Policy here does not allow.
document.body.className += ' js-enabled' + ('noModule' in HTMLScriptElement.prototype ? ' govuk-frontend-supported' : '');
