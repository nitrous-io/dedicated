class Hosts < Thor
  desc "find IDENT", "Searches for host with IDENT. Supports slug, instance id, volume id"
  def find(ident)
    query_field = case ident
    when /\Ai-/
      :provider_identifier
    when /\Avol-/
      :provider_storage_identifier
    else
      :slug
    end
    results = Host.query.where(query_field => ident)
    Host.from_rows(results).each do |h|
      puts h
    end
  end

  desc "by_user IDENT", "Searches for host with owner that has username or e-mail IDENT."
  def by_user(ident)
    results = Host.query.
      where { Sequel.|({username: ident}, {email: ident}) }.
      where(terminated_at: nil)
    puts "Running: #{results.sql}"
    Host.from_rows(results).each do |h|
      puts h
      puts "-" * 10
    end
  end
end

# vim:ft=ruby
