parseSlugs = (slugText) ->
  slugText.split(',').map((slug) ->
    slug.trim())

App.AdminAnnouncementsNewController = Em.Controller.extend
  slugs: null
  subjectTemplate: null
  messageTemplate: null
  expiresAt: null
  showToAll: false
  saving: false

  clear: ->
    @setProperties
      slugs: ''
      subjectTemplate: ''
      messageTemplate: ''
      expiresAt: ''
      showToAll: false
      saving: false

  slugsDisabled: (->
    return @get('showToAll') || @get('saving')
  ).property 'showToAll', 'saving'

  expiryDate: (->
    new Date(@get('expiresAt'))
  ).property 'expiresAt'

  actions:
    sendAnnouncement: ->
      subjectTemplate = @get('subjectTemplate')
      messageTemplate = @get('messageTemplate')
      expiresAt = @get('expiryDate')
      showToAll = @get('showToAll')
      slugs = parseSlugs(@get('slugs'))

      @set 'saving', true

      announcement =
        subject_template: subjectTemplate
        message_template: messageTemplate
        show_to_all: showToAll
        expires_at: expiresAt

      unless showToAll
        announcement.slugs = slugs

      payload =
        announcement: announcement

      App.Model.adapter.ajax('POST', '/admin/announcements/new', JSON.stringify(payload),
        'content-type': 'application/json'
      )
      .done( =>
        App.snackbar.post("Successfully sent announcement")
        @set 'saving', false
        @transitionToRoute 'admin.announcements'

      ).fail (jqxhr) =>
        App.postError(jqxhr.responseJSON, "Failed to send announcement")
        @set 'saving', false
