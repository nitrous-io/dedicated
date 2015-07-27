App.AdminAnnouncementsController = Em.Controller.extend
  announcements: []

  clear: ->
    @setProperties
      announcements: []

    @fetchAnnouncements()

  fetchAnnouncements: ->
    Em.$.ajax(
      type: 'GET'
      url: '/api/v0/admin/announcements/all'
      headers: { Authorization: "Bearer #{App.get 'token'}" }
    )
    .then (data) =>
      @set 'announcements', data.announcements