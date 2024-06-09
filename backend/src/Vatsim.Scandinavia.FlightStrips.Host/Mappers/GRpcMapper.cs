using System.Diagnostics;
using FlightStrips;
using Vatsim.Scandinavia.FlightStrips.Abstractions;
using Vatsim.Scandinavia.FlightStrips.Abstractions.Strips;
using Vatsim.Scandinavia.FlightStrips.Abstractions.Strips.Events;
using CommunicationType = FlightStrips.CommunicationType;
using Position = Vatsim.Scandinavia.FlightStrips.Abstractions.Strips.Position;
using StripState = Vatsim.Scandinavia.FlightStrips.Abstractions.Enums.StripState;
using WeightCategory = FlightStrips.WeightCategory;

namespace Vatsim.Scandinavia.FlightStrips.Host.Mappers;

public class GRpcMapper : IGRpcMapper
{
    public FullStripEvent MapFull(StripData stripData, SessionId session)
    {
        var data = stripData.FullData;
        return new FullStripEvent
        {
            Id = MapStripId(stripData, session),
            Destination = data.Destination,
            Origin = data.Origin,
            Cleared = data.Cleared,
            Route = data.Route,
            Capabilities = Map(data.Capabilities),
            Alternate = data.Alternate,
            AssignedSquawk = data.Squawk,
            Heading = (int?)data.Heading,
            Remarks = data.Remarks,
            Runway = data.Runway,
            Sid = data.Sid,
            State = Map(data.GroundState),
            ClearedAltitude = (int)data.ClearedAlt,
            FinalAltitude = (int)data.FinalAltitude,
            AircraftCategory = Map(data.AircraftCategory),
            CommunicationType = Map(data.CommunicationType),
            TOBT = data.EstimatedDepartureTime,
            AircraftType = data.AircraftType,
            Position = new Position
            {
                Height = (int)data.Position.Altitude,
                Location = new Location(data.Position.Position_.Latitude, data.Position.Position_.Longitude)
            }
        };

    }

    private static FlightStrips.Abstractions.Strips.WeightCategory Map(WeightCategory weightCategory)
    {
        return weightCategory switch
        {
            WeightCategory.Unspecified => Abstractions.Strips.WeightCategory.Unknown,
            WeightCategory.Unknown => Abstractions.Strips.WeightCategory.Unknown,
            WeightCategory.Light => Abstractions.Strips.WeightCategory.Light,
            WeightCategory.Medium => Abstractions.Strips.WeightCategory.Medium,
            WeightCategory.Heavy => Abstractions.Strips.WeightCategory.Heavy,
            WeightCategory.SuperHeavy => Abstractions.Strips.WeightCategory.SuperHeavy,
            _ => Abstractions.Strips.WeightCategory.Unknown
        };

    }

    public PositionEvent MapPosition(StripData stripData, SessionId session)
    {
        return new PositionEvent(MapStripId(stripData, session),
            new Position
            {
                Height = (int)stripData.Position.Position.Altitude,
                Location = new Location(stripData.Position.Position.Position_.Latitude,
                    stripData.Position.Position.Position_.Longitude)
            });
    }

    public StripId MapStripId(StripData stripData, SessionId session)
    {
        return new StripId(session.Airport, session.Session, stripData.Callsign);

    }

    public AircraftCapabilities Map(Capabilities capabilities)
    {
        return capabilities switch
        {
            Capabilities.CapibilitiesUnspecified => AircraftCapabilities.Unknown,
            Capabilities.CapibilitiesUnknown => AircraftCapabilities.Unknown,
            Capabilities.T => AircraftCapabilities.T,
            Capabilities.X => AircraftCapabilities.X,
            Capabilities.U => AircraftCapabilities.U,
            Capabilities.D => AircraftCapabilities.D,
            Capabilities.B => AircraftCapabilities.B,
            Capabilities.A => AircraftCapabilities.A,
            Capabilities.M => AircraftCapabilities.M,
            Capabilities.N => AircraftCapabilities.N,
            Capabilities.P => AircraftCapabilities.P,
            Capabilities.Y => AircraftCapabilities.Y,
            Capabilities.C => AircraftCapabilities.C,
            Capabilities.I => AircraftCapabilities.I,
            Capabilities.E => AircraftCapabilities.E,
            Capabilities.F => AircraftCapabilities.F,
            Capabilities.G => AircraftCapabilities.G,
            Capabilities.R => AircraftCapabilities.R,
            Capabilities.W => AircraftCapabilities.W,
            Capabilities.Q => AircraftCapabilities.Q,
            _ => throw new UnreachableException("Missing enum mapping")
        };
    }

    public StripState Map(GroundState state)
    {
        return state switch
        {
            GroundState.Unspecified => StripState.None,
            GroundState.StartUp => StripState.Startup,
            GroundState.Push => StripState.Push,
            GroundState.Taxi => StripState.Taxi,
            GroundState.DeIce => StripState.Deice,
            GroundState.LineUp => StripState.Lineup,
            GroundState.Depart => StripState.Depart,
            GroundState.Arrival => StripState.Arrival,
            GroundState.None => StripState.None,
            _ => throw new UnreachableException("Missing enum mapping")
        };
    }

    public Vatsim.Scandinavia.FlightStrips.Abstractions.Strips.CommunicationType Map(CommunicationType communicationType)
    {
        return communicationType switch
        {
            CommunicationType.Unspecified => Abstractions.Strips.CommunicationType.Unassigned,
            CommunicationType.Unassigned => Abstractions.Strips.CommunicationType.Unassigned,
            CommunicationType.Voice => Abstractions.Strips.CommunicationType.Voice,
            CommunicationType.Receive => Abstractions.Strips.CommunicationType.Receive,
            CommunicationType.Text => Abstractions.Strips.CommunicationType.Text,
            _ => throw new UnreachableException("Missing enum mapping")
        };
    }

}
