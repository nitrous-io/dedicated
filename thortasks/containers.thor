class Containers < Thor
  desc "info SLUG", "Shows information of a container"
  def info(ident)
    results = Container.query.where(:containers__slug => ident)
    Container.from_rows(results).each do |c|
      puts c
    end
  end

  desc "by_user IDENT", "Searches for containers with owner that has username or e-mail IDENT."
  def by_user(ident)
    results = Container.query.
      where { Sequel.|({users__username: ident}, {users__email: ident}) }.
      where(containers__terminated_at: nil)
    Container.from_rows(results).each do |c|
      puts c
      puts "-" * 10
    end
  end

  desc "by_host SLUG", "Searches for containers with host that has slug SLUG."
  def by_host(ident)
    results = Container.query.
      where { Sequel.|({hosts__slug: ident})}.
      where(containers__terminated_at: nil)
    Container.from_rows(results).each do |c|
      puts c
      puts "-" * 10
    end
  end
end
