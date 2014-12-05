window.App ||= {}

class App.TeamPage
  constructor: ->
    $("[rel='tooltip']").tooltip(html: true)
    $('.carousel').carousel(interval: 5000)
