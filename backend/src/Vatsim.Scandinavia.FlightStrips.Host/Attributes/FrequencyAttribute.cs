using System.ComponentModel.DataAnnotations;
using System.Diagnostics.CodeAnalysis;

namespace Vatsim.Scandinavia.FlightStrips.Host.Attributes;

[AttributeUsage(AttributeTargets.Property | AttributeTargets.Field | AttributeTargets.Parameter, AllowMultiple = false)]
public class FrequencyAttribute : RegularExpressionAttribute
{
    [StringSyntax(StringSyntaxAttribute.Regex)]
    private const string Regex = @"^\d{3}\.\d{3}$";

    public FrequencyAttribute() : base(Regex)
    {
    }
}
