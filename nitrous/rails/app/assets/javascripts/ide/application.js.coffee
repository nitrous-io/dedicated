#= require jquery
#= require jquery_ujs
#= require bootstrap
#= require ./vendor/underscore
#= require ./vendor/modernizr-html5-2.6.2
#= require ./vendor/json2
#= require_self
#= require ./license
#= require ./vendor/placeholder_polyfill.jquery

if location.hash == '#aio'
  # show name change notice
  $div = $('<div>').attr('id', 'name-change-notice').addClass('top-announcement container').html("<p><i class='fa fa-bullhorn'></i> Action.IO is now Nitrous! <a href='http://blog.nitrous.io/2013/04/16/actionio-is-now-nitrousio.html'>Read more...</a></p>")
  $('body').append($div)
  window.setTimeout(->
    $div.fadeIn()
  , 200)
  $div.click (e) ->
    $div.fadeOut() if e.target.tagName.toLowerCase() == 'p'

$ ->
  window.setTimeout(->
    $alert = $('#flash_messages .alert')
    if $('span', $alert).text().length > 0
      $alert.addClass 'in'
  , 500)

  $(window).on 'message', (event) ->
    if event.originalEvent.data == "chromeWebView"
      window.App?.set? 'isChromeWebView', true
      $('#page-header, #header-bottom').hide()

  $('body').on 'blur', '.mailcheck', (event) ->
    _removeSuggest = =>
      $(this).nextAll('#mail-suggest').remove()

    _removeSuggest()

    $(this).mailcheck
      # Copied from app/helpers/users/registrations_helper.rb
      domains: ["gmail.com", "googlemail.com", "yahoo.com", "yahoomail.com", "ymail.com", "yahoo.ca", "yahoo.cn", "yahoo.co.id", "yahoo.co.in", "yahoo.co.jp", "yahoo.co.kr", "yahoo.co.nz", "yahoo.co.uk", "yahoo.de", "yahoo.dk", "yahoo.es", "yahoo.fr", "yahoo.gr", "yahoo.ie", "yahoo.in", "yahoo.it", "yahoo.pl", "yahoo.ro", "yahoo.ru", "yahoo.se", "yahoo.com.ar", "yahoo.com.au", "yahoo.com.br", "yahoo.com.cn", "yahoo.com.hk", "yahoo.com.mx", "yahoo.com.my", "yahoo.com.ph", "yahoo.com.sg", "yahoo.com.tw", "yahoo.com.vn", "hotmail.com", "live.com", "hotmail.co.uk", "hotmail.co.jp", "msn.com", "me.com", "mac.com", "icloud.com", "naver.com", "hanmail.net", "daum.net", "aol.com", "comcast.net", "facebook.com", "verizon.net", "gmx.com", "gmx.net", "gmx.de", "mail.com", "fastmail.fm", "yandex.com", "yandex.com.tr", "yandex.by", "yandex.ru", "yandex.ua"]
      topLevelDomains: ["co.jp", "co.uk", "com", "net", "org", "info", "edu", "gov", "mil", "ru", "de", "dk", "es", "fr", "gr", "ie", "in", "pl", "ro", "se", "ca", "cn", "co.id", "co.in", "co.kr", "co.nz", "fm", "io", "ua", "com.tr"]

      suggested: ((element, suggestion) ->

        suggestField = $('<div id="mail-suggest"></div>').hide()
        suggestField.html "Did you mean <a>#{suggestion.full}</a>?"

        suggestField.on 'click', (event) ->
          element.val suggestion.full
          _removeSuggest()
          mixpanel.track('Clicked On E-Mail Suggestion') if mixpanel?
          event.preventDefault()

        # need this hack to align suggest block
        # in invites page
        button = element.next('button')
        if button.length
          button.after suggestField
        else
          element.after suggestField

        suggestField.slideDown('fast')
        mixpanel.track('Showed E-Mail Suggestion') if mixpanel?
      )
