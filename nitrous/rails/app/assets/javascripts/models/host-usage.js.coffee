#= require ./model
attr = SM.attr

App.HostUsage = App.Model.extend
  minutes: attr('number', as: 'minutes')
  startAt: attr('date', as: 'start_at')
  endAt: attr('date', as: 'end_at')

  hours: (->
    @get('minutes') / 60.0
  ).property 'minutes'