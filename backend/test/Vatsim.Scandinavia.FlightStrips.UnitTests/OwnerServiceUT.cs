using Microsoft.Extensions.Logging;
using NSubstitute;
using Vatsim.Scandinavia.FlightStrips.Abstractions.OnlinePositions;
using Vatsim.Scandinavia.FlightStrips.Abstractions.Positions;
using Vatsim.Scandinavia.FlightStrips.Abstractions.Runways;
using Vatsim.Scandinavia.FlightStrips.Abstractions.Sectors;
using Vatsim.Scandinavia.FlightStrips.Services;
using Vatsim.Scandinavia.FlightStrips.Abstractions;

namespace Vatsim.Scandinavia.FlightStrips.UnitTests;

public class OwnerServiceUT
{
    [Test]
    public void GetOwnersSetsAllOwnersAllSectors()
    {
        // Arrange
        var runwayConfig = new RunwayConfig("22R", "22L", string.Empty);
        var onlinePositions = new OnlinePosition[]
        {
            new()
            {
                Id = new OnlinePositionId("EKCH", "LIVE", "EKCH_A_TWR"),
                PrimaryFrequency = Position.EKCH_A_TWR.Frequency,
                Sector = Sector.NONE
            },
            new()
            {
                Id = new OnlinePositionId("EKCH", "LIVE", "EKCH_DEL"),
                PrimaryFrequency = Position.EKCH_DEL.Frequency,
                Sector = Sector.NONE
            }
        };

        var session = new SessionId("EKCH", "LIVE");

#pragma warning disable CA1859
        IOwnerService service = new OwnerService(Substitute.For<ILogger<OwnerService>>());
#pragma warning restore CA1859

        // Act
        var result = service.GetOwners(session, runwayConfig, onlinePositions);

        // Assert
        Assert.Multiple(() =>
        {
            Assert.That(result[0].Sector, Is.EqualTo(Sectors.All ^ Sector.DEL));
            Assert.That(result[1].Sector, Is.EqualTo(Sector.DEL));
        });
    }

    [Test]
    public void GetOwnersSetsAllOwnersOnlyOnline()
    {
        // Arrange
        var runwayConfig = new RunwayConfig("22R", "22L", string.Empty);
        var onlinePositions = new OnlinePosition[]
        {
            new()
            {
                Id = new OnlinePositionId("EKCH", "LIVE", "EKCH_A_GND"),
                PrimaryFrequency = Position.EKCH_A_GND.Frequency,
                Sector = Sector.NONE
            },
            new()
            {
                Id = new OnlinePositionId("EKCH", "LIVE", "EKCH_DEL"),
                PrimaryFrequency = Position.EKCH_DEL.Frequency,
                Sector = Sector.NONE
            }
        };

        var session = new SessionId("EKCH", "LIVE");

#pragma warning disable CA1859
        IOwnerService service = new OwnerService(Substitute.For<ILogger<OwnerService>>());
#pragma warning restore CA1859

        // Act
        var result = service.GetOwners(session, runwayConfig, onlinePositions);

        // Assert
        Assert.Multiple(() =>
        {
            Assert.That(result[0].Sector, Is.EqualTo(Sector.AA | Sector.AD));
            Assert.That(result[1].Sector, Is.EqualTo(Sector.DEL));
        });
    }

    [Test]
    public void OnlyOneControllerAllSectors()
    {
        // Arrange
        var all = Sectors.All.ToString();

        var runwayConfig = new RunwayConfig("22R", "22L", string.Empty);
        var onlinePositions = new OnlinePosition[]
        {
            new()
            {
                Id = new OnlinePositionId("EKCH", "LIVE", "EKCH_A_TWR"),
                PrimaryFrequency = Position.EKCH_A_TWR.Frequency,
                Sector = Sector.NONE
            },
        };

        var session = new SessionId("EKCH", "LIVE");

#pragma warning disable CA1859
        IOwnerService service = new OwnerService(Substitute.For<ILogger<OwnerService>>());
#pragma warning restore CA1859

        // Act
        var result = service.GetOwners(session, runwayConfig, onlinePositions);

        // Assert
        Assert.That(result, Has.Length.EqualTo(1));
        Assert.That(result[0].Sector.ToString(), Is.EqualTo(all));
    }
}
