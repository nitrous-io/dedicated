#= require jquery
#= require application
#= require qunit-bdd-0.6.1
#= require sinon-1.10.3
#= require main
#= require_self
#= require_tree .

window.beforeEach = window.before
window.afterEach = window.after

window.expect.configure Object.defineProperties {},
  instanceof:
    value: (expected, message) ->
      if @_flags.negate
        QUnit.strictEqual(@_actual instanceof expected, false, message)
      else
        QUnit.strictEqual(@_actual instanceof expected, true, message)

  contain:
    value: (expected, message) ->
      if @_flags.negate
        QUnit.strictEqual(@_actual.indexOf(expected), -1, message)
      else
        QUnit.notStrictEqual(@_actual.indexOf(expected), -1, message)

  haveLength:
    value: (expected, message) ->
      if @_flags.negate
        QUnit.notStrictEqual(@_actual.length, expected, message)
      else
        QUnit.strictEqual(@_actual.length, expected, message)

# expected global vars
[
  'XMLHttpRequest'
].forEach (s) ->
  window[s] = window[s]

App.setupForTesting()
App.injectTestHelpers()

Em.Test.adapter = Em.Test.QUnitAdapter.create()

window.Test =
  nextTick: (next) ->
    QUnit.stop()
    window.setTimeout(->
      QUnit.start()
      next()
    , 0)

  responseInternalServerError: [
    500
    { 'Content-Type': 'application/json; charset=utf-8' }
    '{"error": "internal server error"}'
  ]

$ ->
  $('body').append($('<div>').attr(id: 'app').css(display: 'none'))
