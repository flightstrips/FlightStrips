using Microsoft.EntityFrameworkCore;
using Microsoft.EntityFrameworkCore.Diagnostics;
using Vatsim.Scandinavia.FlightStrips.Abstractions;

namespace Vatsim.Scandinavia.FlightStrips.Persistence.EfCore;

public class TenantSaveChangesInterceptor : SaveChangesInterceptor
{
    private readonly ITenantService _tenantService;

    public TenantSaveChangesInterceptor(ITenantService tenantService)
    {
        _tenantService = tenantService;
    }

    public override InterceptionResult<int> SavingChanges(DbContextEventData eventData, InterceptionResult<int> result)
    {
        if (eventData.Context is null)
        {
            return result;
        }

        SetTenantValues(eventData.Context);

        return result;
    }

    public override ValueTask<InterceptionResult<int>> SavingChangesAsync(DbContextEventData eventData,
        InterceptionResult<int> result,
        CancellationToken cancellationToken = default)
    {
        if (eventData.Context is null)
        {
            return ValueTask.FromResult(result);
        }

        SetTenantValues(eventData.Context);

        return ValueTask.FromResult(result);
    }


    private void SetTenantValues(DbContext context)
    {
        var changeTracker = context.ChangeTracker;

        foreach (var entry in changeTracker.Entries<IAirportTenant>().Where(x => x.State == EntityState.Added))
        {
            entry.Entity.Airport = _tenantService.Airport;
        }

        foreach (var entry in changeTracker.Entries<ISessionTenant>().Where(x => x.State == EntityState.Added))
        {
            entry.Entity.Session = _tenantService.Session;
        }

    }
}
