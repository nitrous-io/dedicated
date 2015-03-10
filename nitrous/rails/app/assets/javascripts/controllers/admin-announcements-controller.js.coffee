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
      console.log('got announcements', data)
      @set 'announcements', data.announcements