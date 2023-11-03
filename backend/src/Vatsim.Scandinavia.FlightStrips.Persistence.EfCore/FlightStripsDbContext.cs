using System.Linq.Expressions;
using System.Reflection;
using Microsoft.EntityFrameworkCore;
using Vatsim.Scandinavia.FlightStrips.Abstractions;
using Vatsim.Scandinavia.FlightStrips.Persistence.EfCore.Entities;

namespace Vatsim.Scandinavia.FlightStrips.Persistence.EfCore;

public class FlightStripsDbContext : DbContext
{
    private readonly ITenantService _tenantService;
    private readonly TenantSaveChangesInterceptor _tenantSaveChangesInterceptor;

    public DbSet<BayEntity> Bays { get; set; } = null!;
    public DbSet<StripEntity> Strips { get; set; } = null!;
    public DbSet<PositionEntity> Positions { get; set; } = null!;
    public DbSet<OnlinePositionEntity> OnlinePositions { get; set; } = null!;

    public DbSet<CoordinationEntity> Coordination { get; set; } = null!;

    public FlightStripsDbContext(DbContextOptions<FlightStripsDbContext> options, ITenantService tenantService) : base(options)
    {
        _tenantService = tenantService;
        _tenantSaveChangesInterceptor = new TenantSaveChangesInterceptor(_tenantService);
    }

    protected override void OnConfiguring(DbContextOptionsBuilder optionsBuilder)
    {
        optionsBuilder.AddInterceptors(_tenantSaveChangesInterceptor);
    }

    protected override void OnModelCreating(ModelBuilder modelBuilder)
    {
        foreach (var entity in modelBuilder.Model.GetEntityTypes()
                     .Where(x => typeof(IAirportAndSessionTenant).IsAssignableFrom(x.ClrType)))
        {
            entity.SetQueryFilter(CreateFilter(entity.ClrType, nameof(AddAirportAndSessionFilter)));
        }


        foreach (var entity in modelBuilder.Model.GetEntityTypes()
                     .Where(x => !typeof(IAirportAndSessionTenant).IsAssignableFrom(x.ClrType) &&
                                 typeof(IAirportTenant).IsAssignableFrom(x.ClrType)))
        {
            entity.SetQueryFilter(CreateFilter(entity.ClrType, nameof(AddAirportFilter)));
        }

        foreach (var entity in modelBuilder.Model.GetEntityTypes()
                     .Where(x => !typeof(IAirportAndSessionTenant).IsAssignableFrom(x.ClrType) &&
                                 typeof(ISessionTenant).IsAssignableFrom(x.ClrType)))
        {
            entity.SetQueryFilter(CreateFilter(entity.ClrType, nameof(AddSessionFilter)));
        }
    }

    private LambdaExpression CreateFilter(Type type, string methodName)
    {
        var genericMethod =
            typeof(FlightStripsDbContext).GetMethod(methodName, BindingFlags.NonPublic | BindingFlags.Instance);
        if (genericMethod is null)
        {
            throw new InvalidOperationException($"Method {methodName} not found");
        }

        var method = genericMethod.MakeGenericMethod(type);

        var result = method.Invoke(this, null);

        if (result is LambdaExpression expression)
        {
            return expression;
        }

        throw new InvalidOperationException("Result is either null or unable to be cast to LambdaExpression.");
    }

    private LambdaExpression AddAirportAndSessionFilter<T>() where T : IAirportAndSessionTenant
    {
        return (T x) => x.Airport == _tenantService.Airport && x.Session == _tenantService.Session;
    }

    private LambdaExpression AddAirportFilter<T>() where T : IAirportTenant
    {
        return (T x) => x.Airport == _tenantService.Airport;
    }

    private LambdaExpression AddSessionFilter<T>() where T : ISessionTenant
    {
        return (T x) => x.Session == _tenantService.Session;
    }
}
