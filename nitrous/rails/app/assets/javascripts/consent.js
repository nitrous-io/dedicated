//= require querystring
//= require es6-promise
//= require fetch

document.addEventListener("DOMContentLoaded", function() {
  'use strict'

  var querystring = window.querystring
  var fetch = window.fetch

  var clientEl = document.getElementById('client-info')
  var clientNameEl = document.getElementById('client-name')
  var usernameEl = document.getElementById('username')

  var tokenField = document.getElementById('access-token')
  var stateField = document.getElementById('state')
  var clientIdField = document.getElementById('client-id')
  var redirectUriField = document.getElementById('redirect-uri')

  // Valid error codes accepted by OAuth2 specs
  var validErrors = [
    'invalid_request', 'unauthorized_client', 'access_denied',
    'unsupported_response_type', 'invalid_scope', 'server_error',
    'temporarily_unavailable'
  ]

  var ajax = function(url, params) {
    return fetch(url, params)
      .then(function(response) {
        if(response.status != 200) {
          return response.json().then(function(json) {
            return Promise.reject(json)
          })
        }

        return response.json()
      })
  }

  var validateRedirectUri = function(originalUri, providedUri) {
    if(!providedUri) return true

    // fragment not allowed
    if(providedUri.indexOf('#') != -1) return false

    return originalUri.split('?')[0] === providedUri.split('?')[0]
  }

  var appendQueryString = function(url, params) {
    var stringified = querystring.stringify(params)

    if(url.indexOf('?') == -1) {
      return url + '?' + stringified
    }

    if(url[url.length-1] != '&') {
      url += '&'
    }

    return url + stringified
  }

  var redirect = function(url, params) {
    if(params) {
      url = appendQueryString(url, params)
    }

    window.location = url
  }

  var redirectError = function(uri, err) {
    var errorCode = err.error || 'server_error'
    var description = err.error_description ||
      'unexpected server error'

    if(validErrors.indexOf(errorCode) == -1) {
      errorCode = 'server_error'
    }

    redirect(uri, {
      error: errorCode,
      error_description: description
    })
  }

  var fetchClient = function(token, clientId) {
    var url = '/api/v0/oauth/clients/' + clientId

    return ajax(url, {
      method: 'GET',
      headers: {
        Authorization: 'Bearer ' + token
      }
    }).then(function(data) {
      return data.oauth_client
    })
  }

  var fetchUser = function(token) {
    return ajax('/api/v0/user', {
      method: 'GET',
      headers: {
        Authorization: 'Bearer ' + token
      }
    }).then(function(data) {
      return data.user
    })
  }

  var showConsentPage = function(token, query, client, user) {
    var redirectUri = query.redirect_uri

    stateField.value = query.state
    clientIdField.value = client.client_id
    redirectUriField.value = query.redirect_uri

    clientNameEl.textContent = client.name
    usernameEl.textContent = user.username
    clientEl.classList.remove('hide')
  }

  var query = querystring.parse(location.search)
  var clientId = query.client_id

  if(!clientId) redirect('/app')

  var token = localStorage.getItem('token')
  if(!token) {
    // TODO: either implement login page here as well
    // or make app#/login redirect back to this page
    return redirect('/app#/login')
  }

  tokenField.value = token

  Promise.all([
    fetchClient(token, clientId),
    fetchUser(token)
  ])
  .then(function(res) {
    var client = res[0]
    var user = res[1]

    if(!validateRedirectUri(client.redirect_uri, query.redirect_uri)) {
      // return redirect('/404')
    }

    return showConsentPage(token, query, client, user)
  }, function(err) {
    // TODO: display more informative page
    // if client cannot be found or error fetching
    redirect('/404')
  })
  .catch(function(err) {
    console.error(err)
  })

})
