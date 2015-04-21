Rails.application.routes.draw do
  get '/app' => 'web_app#index'
  get '/ide' => 'web_app#ide'
  get '/features' => 'pages#features'
  get '/pricing' => 'pages#pricing'
  get '/company' => 'pages#company'
  get '/press' => 'pages#press'
  get '/plan_changes' => 'pages#plan_changes'
  get '/whatsnew' => 'pages#whatsnew'

  get '/education' => 'pages#education'
  get '/agencies' => 'pages#agencies'
  get '/enterprises' => 'pages#enterprises'
  get '/teams' => 'pages#teams'
  get '/vpc' => 'pages#vpc'

  get '/stacks' => 'pages#stacks'
  get '/stacks/:stack' => 'pages#stack'

  root 'pages#home'
end
